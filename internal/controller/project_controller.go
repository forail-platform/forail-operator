package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	forailv1 "github.com/forail-platform/forail-operator/api/v1alpha1"
	"github.com/forail-platform/forail-operator/internal/forailapi"
)

const projectFinalizer = "project.forail.forail-platform.io/finalizer"

// ProjectReconciler reconciles a Project CR with Forail.
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Forail is the default client (global FORAIL_URL/TOKEN). Used when
	// spec.forailInstance is empty.
	Forail *forailapi.Client
	// Pool dispenses per-ForailInstance clients for multi-cluster CRs.
	// Nil pool falls back to Forail.
	Pool *forailapi.ClientPool
}

// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=projects/finalizers,verbs=update

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr forailv1.Project
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !cr.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &cr)
	}

	if !hasFinalizer(cr.Finalizers, projectFinalizer) {
		cr.Finalizers = append(cr.Finalizers, projectFinalizer)
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	fc, err := clientFor(ctx, r.Pool, r.Forail, cr.Namespace, cr.Spec.ForailInstance)
	if err != nil {
		return r.markProjectErr(ctx, &cr, reasonResolveErr, fmt.Errorf("forail instance: %w", err))
	}

	desired, err := r.buildDesired(ctx, fc, &cr)
	if err != nil {
		return r.markProjectErr(ctx, &cr, reasonResolveErr, err)
	}

	current, err := r.findExisting(ctx, fc, &cr, desired.Name)
	if err != nil {
		return r.markProjectErr(ctx, &cr, reasonAPIError, err)
	}

	if current == nil {
		created, err := fc.CreateProject(ctx, desired)
		if err != nil {
			return r.markProjectErr(ctx, &cr, reasonAPIError, fmt.Errorf("create: %w", err))
		}
		logger.Info("created Project in Forail", "id", created.ID, "name", created.Name)
		current = created
	} else if !equalProject(current, desired) {
		updated, err := fc.UpdateProject(ctx, current.ID, desired)
		if err != nil {
			return r.markProjectErr(ctx, &cr, reasonAPIError, fmt.Errorf("update: %w", err))
		}
		logger.Info("updated Project in Forail", "id", updated.ID)
		current = updated
	}

	cr.Status.ForailID = current.ID
	cr.Status.ObservedGeneration = cr.Generation
	cr.Status.ScmRevision = current.ScmRevision
	setProjectCondition(&cr, conditionSynced, metav1.ConditionTrue, reasonInSync, "Project is in sync with Forail")
	setProjectCondition(&cr, conditionReady, metav1.ConditionTrue, reasonInSync, "")
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *ProjectReconciler) reconcileDelete(ctx context.Context, cr *forailv1.Project) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if cr.Status.ForailID > 0 {
		fc, ferr := clientFor(ctx, r.Pool, r.Forail, cr.Namespace, cr.Spec.ForailInstance)
		if ferr != nil {
			return ctrl.Result{}, fmt.Errorf("resolve forail instance for delete: %w", ferr)
		}
		if err := fc.DeleteProject(ctx, cr.Status.ForailID); err != nil && !forailapi.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete forail Project %d: %w", cr.Status.ForailID, err)
		}
		logger.Info("deleted Project from Forail", "id", cr.Status.ForailID)
	}
	cr.Finalizers = removeString(cr.Finalizers, projectFinalizer)
	return ctrl.Result{}, r.Update(ctx, cr)
}

func (r *ProjectReconciler) buildDesired(ctx context.Context, fc *forailapi.Client, cr *forailv1.Project) (*forailapi.Project, error) {
	name := cr.Spec.Name
	if name == "" {
		name = cr.Name
	}

	orgID, err := fc.ResolveOrganization(ctx, cr.Spec.Organization)
	if err != nil {
		return nil, fmt.Errorf("resolve organization %q: %w", cr.Spec.Organization, err)
	}
	if orgID < 0 {
		return nil, fmt.Errorf("organization %q not found in Forail", cr.Spec.Organization)
	}

	scmType := cr.Spec.ScmType
	if scmType == "" {
		scmType = "git"
	}
	if scmType == "manual" {
		scmType = ""
	}

	p := &forailapi.Project{
		Name:                  name,
		Description:           cr.Spec.Description,
		Organization:          orgID,
		ScmType:               scmType,
		ScmURL:                cr.Spec.ScmURL,
		ScmBranch:             cr.Spec.ScmBranch,
		ScmRefspec:            cr.Spec.ScmRefspec,
		ScmClean:              cr.Spec.ScmClean,
		ScmDeleteOnUpdate:     cr.Spec.ScmDeleteOnUpdate,
		ScmUpdateOnLaunch:     cr.Spec.ScmUpdateOnLaunch,
		ScmUpdateCacheTimeout: cr.Spec.ScmUpdateCacheTimeout,
		AllowOverride:         cr.Spec.AllowOverride,
		Timeout:               cr.Spec.Timeout,
	}

	if cr.Spec.ScmCredential != "" {
		credID, err := fc.ResolveCredential(ctx, cr.Spec.ScmCredential)
		if err != nil {
			return nil, fmt.Errorf("resolve credential %q: %w", cr.Spec.ScmCredential, err)
		}
		if credID < 0 {
			return nil, fmt.Errorf("credential %q not found in Forail", cr.Spec.ScmCredential)
		}
		p.Credential = &credID
	}

	if cr.Spec.DefaultEnvironment != "" {
		eeID, err := fc.ResolveExecutionEnvironment(ctx, cr.Spec.DefaultEnvironment)
		if err != nil {
			return nil, fmt.Errorf("resolve execution_environment %q: %w", cr.Spec.DefaultEnvironment, err)
		}
		if eeID < 0 {
			return nil, fmt.Errorf("execution_environment %q not found in Forail", cr.Spec.DefaultEnvironment)
		}
		p.DefaultEnvironment = &eeID
	}

	return p, nil
}

func (r *ProjectReconciler) findExisting(ctx context.Context, fc *forailapi.Client, cr *forailv1.Project, name string) (*forailapi.Project, error) {
	if cr.Status.ForailID > 0 {
		p, err := fc.GetProject(ctx, cr.Status.ForailID)
		if err == nil {
			return p, nil
		}
		if !forailapi.IsNotFound(err) {
			return nil, err
		}
	}
	return fc.FindProjectByName(ctx, name)
}

func (r *ProjectReconciler) markProjectErr(ctx context.Context, cr *forailv1.Project, reason string, err error) (ctrl.Result, error) {
	setProjectCondition(cr, conditionReady, metav1.ConditionFalse, reason, err.Error())
	setProjectCondition(cr, conditionSynced, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, cr); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forailv1.Project{}).
		Complete(r)
}

// --- helpers ---

func setProjectCondition(cr *forailv1.Project, condType string, status metav1.ConditionStatus, reason, msg string) {
	now := metav1.Now()
	for i, c := range cr.Status.Conditions {
		if c.Type == condType {
			if c.Status != status {
				cr.Status.Conditions[i].LastTransitionTime = now
			}
			cr.Status.Conditions[i].Status = status
			cr.Status.Conditions[i].Reason = reason
			cr.Status.Conditions[i].Message = msg
			cr.Status.Conditions[i].ObservedGeneration = cr.Generation
			return
		}
	}
	cr.Status.Conditions = append(cr.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: now,
		ObservedGeneration: cr.Generation,
	})
}

func equalProject(a, b *forailapi.Project) bool {
	return a.Name == b.Name &&
		a.Description == b.Description &&
		a.Organization == b.Organization &&
		a.ScmType == b.ScmType &&
		a.ScmURL == b.ScmURL &&
		a.ScmBranch == b.ScmBranch &&
		a.ScmRefspec == b.ScmRefspec &&
		equalInt64Ptr(a.Credential, b.Credential) &&
		a.ScmClean == b.ScmClean &&
		a.ScmDeleteOnUpdate == b.ScmDeleteOnUpdate &&
		a.ScmUpdateOnLaunch == b.ScmUpdateOnLaunch &&
		a.ScmUpdateCacheTimeout == b.ScmUpdateCacheTimeout &&
		a.AllowOverride == b.AllowOverride &&
		a.Timeout == b.Timeout &&
		equalInt64Ptr(a.DefaultEnvironment, b.DefaultEnvironment)
}

func equalInt64Ptr(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
