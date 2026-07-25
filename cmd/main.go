package main

import (
	"flag"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	forailv1 "github.com/forail-platform/forail-operator/api/v1alpha1"
	"github.com/forail-platform/forail-operator/internal/controller"
	"github.com/forail-platform/forail-operator/internal/forailapi"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(forailv1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		probeAddr            string
		enableLeaderElection bool
		forailURL        string
		forailToken      string
		forailHostHeader string
		forailInsecure   bool
		secretNamespaces string
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Metrics endpoint")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe endpoint")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election")
	flag.StringVar(&forailURL, "forail-url", os.Getenv("FORAIL_URL"), "Forail API base URL (e.g. https://forail-web.forail.svc.cluster.local:8013)")
	flag.StringVar(&forailToken, "forail-token", os.Getenv("FORAIL_TOKEN"), "Forail OAuth2 personal access token (Bearer)")
	flag.StringVar(&forailHostHeader, "forail-host-header", os.Getenv("FORAIL_HOST_HEADER"), "Host header to send (when reaching Forail via host-routed Ingress)")
	flag.BoolVar(&forailInsecure, "forail-insecure-skip-verify", os.Getenv("FORAIL_INSECURE") == "true", "Skip TLS verify on Forail API (test only)")
	flag.StringVar(&secretNamespaces, "secret-namespaces", os.Getenv("SECRET_NAMESPACES"), "Comma-separated extra namespaces to read Credential Secrets from, on top of the operator's own. Each one also needs the Secret Role the chart creates (secretNamespaces in values.yaml)")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if forailURL == "" || forailToken == "" {
		setupLog.Info("no default Forail backend configured; CRs without spec.forailInstance will not reconcile until --forail-url and --forail-token (or FORAIL_URL / FORAIL_TOKEN env) are set")
	}

	// needtofix M12 follow-up: the credential reconciler watches Secrets, and the
	// manager's default cache watches them at CLUSTER scope. Since M12 narrowed the
	// Secret grant to a namespaced Role, that informer can never list — it fails
	// permanently, the shared cache never reports synced, and the manager exits
	// ("failed to wait for ... caches to sync"), taking every controller down with
	// it. Scope the Secret cache to the operator's own namespace so the informer
	// asks only for what the Role actually grants.
	//
	// A Credential resolves spec.inputsFrom in its OWN namespace, so the
	// operator's namespace alone only serves Credentials that live beside it.
	// --secret-namespaces widens that to a named set. The Role grants and this
	// cache scope have to move together: a RoleBinding in another namespace
	// without the namespace here still fails with "unknown namespace for the
	// cache", and this list without the Role gets a 403 the cache never
	// recovers from. The chart drives both from one values key.
	cacheOpts := cache.Options{}
	if nss := secretCacheNamespaces(operatorNamespace(), secretNamespaces); len(nss) > 0 {
		byNamespace := make(map[string]cache.Config, len(nss))
		for _, ns := range nss {
			byNamespace[ns] = cache.Config{}
		}
		cacheOpts.ByObject = map[client.Object]cache.ByObject{
			&corev1.Secret{}: {Namespaces: byNamespace},
		}
		setupLog.Info("scoping the Secret cache", "namespaces", nss)
	} else {
		// Out-of-cluster runs (`make run`) use a kubeconfig that is normally
		// cluster-admin, so an unscoped Secret cache is correct there.
		setupLog.Info("operator namespace unknown (no POD_NAMESPACE env, no service-account namespace file); caching Secrets cluster-wide")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "forail-operator-leader.forail.forail-platform.io",
		Cache:                  cacheOpts,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	forailClient := forailapi.New(forailURL, forailToken, forailHostHeader, forailInsecure)
	pool := forailapi.NewClientPool(forailClient, mgr.GetClient())

	if err := (&controller.ForailInstanceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Pool:   pool,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up ForailInstance controller")
		os.Exit(1)
	}

	if err := (&controller.JobTemplateReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up JobTemplate controller")
		os.Exit(1)
	}

	if err := (&controller.InventoryReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Inventory controller")
		os.Exit(1)
	}

	if err := (&controller.CredentialReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Credential controller")
		os.Exit(1)
	}

	if err := (&controller.ScheduleReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Schedule controller")
		os.Exit(1)
	}

	if err := (&controller.ProjectReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
		Pool:   pool,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Project controller")
		os.Exit(1)
	}

	if err := (&controller.OrganizationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
		Pool:   pool,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Organization controller")
		os.Exit(1)
	}

	if err := (&controller.TeamReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
		Pool:   pool,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Team controller")
		os.Exit(1)
	}

	if err := (&controller.WorkflowReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Forail:  forailClient,
		Pool:   pool,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up Workflow controller")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "forailURL", forailURL)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager exited with error")
		os.Exit(1)
	}
}

// operatorNamespace resolves the namespace the operator runs in: the
// POD_NAMESPACE downward-API env the chart sets, falling back to the namespace
// file every in-cluster ServiceAccount is mounted with (so a manifest that
// predates the env var still scopes correctly). Empty means out-of-cluster.
// secretCacheNamespaces returns the namespaces the Secret cache should cover:
// the operator's own plus the comma-separated extras, trimmed and deduplicated,
// keeping the caller's order so the logged list is stable.
//
// Returns nil when the operator namespace is unknown AND no extras were given —
// that is the out-of-cluster case, where an unscoped cache is correct.
func secretCacheNamespaces(own, extra string) []string {
	own = strings.TrimSpace(own)
	if own == "" {
		// Out-of-cluster (`make run`): scoping to a set that cannot include the
		// operator's own namespace would be worse than not scoping at all.
		return nil
	}
	out := []string{own}
	seen := map[string]bool{own: true}
	for _, ns := range strings.Split(extra, ",") {
		ns = strings.TrimSpace(ns)
		if ns == "" || seen[ns] {
			continue
		}
		seen[ns] = true
		out = append(out, ns)
	}
	return out
}

func operatorNamespace() string {
	if ns := strings.TrimSpace(os.Getenv("POD_NAMESPACE")); ns != "" {
		return ns
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
