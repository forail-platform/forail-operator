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

	forgev1 "github.com/forgeplatform/forge-operator/api/v1alpha1"
	"github.com/forgeplatform/forge-operator/internal/forgeapi"
)

const (
	finalizer = "jobtemplate.forge.forgeplatform.io/finalizer"

	conditionReady  = "Ready"
	conditionSynced = "Synced"

	reasonReconciling = "Reconciling"
	reasonResolveErr  = "ResolveError"
	reasonAPIError    = "ForgeAPIError"
	reasonInSync      = "InSync"
)

// JobTemplateReconciler reconciles a JobTemplate CR with Forge.
type JobTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Forge  *forgeapi.Client
}

// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=jobtemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=jobtemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=jobtemplates/finalizers,verbs=update

func (r *JobTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr forgev1.JobTemplate
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion: if the CR is being deleted, run finalizer to clean up
	// the corresponding JobTemplate in Forge before allowing the CR to go.
	if !cr.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &cr)
	}

	if !controllerHasFinalizer(&cr) {
		cr.Finalizers = append(cr.Finalizers, finalizer)
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Build the desired Forge representation, resolving name refs.
	desired, err := r.buildDesired(ctx, &cr)
	if err != nil {
		return r.markError(ctx, &cr, reasonResolveErr, err)
	}

	// Find existing in Forge — by ID first (status), fallback to name lookup
	// so we adopt resources created out of band by the same name.
	current, err := r.findExisting(ctx, &cr, desired.Name)
	if err != nil {
		return r.markError(ctx, &cr, reasonAPIError, err)
	}

	if current == nil {
		created, err := r.Forge.CreateJobTemplate(ctx, desired)
		if err != nil {
			return r.markError(ctx, &cr, reasonAPIError, fmt.Errorf("create: %w", err))
		}
		logger.Info("created JobTemplate in Forge", "id", created.ID, "name", created.Name)
		current = created
	} else {
		// Patch only if drift detected. PATCH a partial doc is idempotent.
		if !equalJobTemplate(current, desired) {
			updated, err := r.Forge.UpdateJobTemplate(ctx, current.ID, desired)
			if err != nil {
				return r.markError(ctx, &cr, reasonAPIError, fmt.Errorf("update: %w", err))
			}
			logger.Info("updated JobTemplate in Forge", "id", updated.ID)
			current = updated
		}
	}

	// Sync credential associations (M2M relation, separate endpoint).
	if err := r.syncCredentials(ctx, &cr, current.ID); err != nil {
		return r.markError(ctx, &cr, reasonAPIError, fmt.Errorf("credentials: %w", err))
	}

	// Status update: ID + ObservedGeneration + Conditions.
	cr.Status.ForgeID = current.ID
	cr.Status.ObservedGeneration = cr.Generation
	setCondition(&cr, conditionSynced, metav1.ConditionTrue, reasonInSync, "JobTemplate is in sync with Forge")
	setCondition(&cr, conditionReady, metav1.ConditionTrue, reasonInSync, "")
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Periodic re-reconcile to detect external drift in Forge.
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *JobTemplateReconciler) reconcileDelete(ctx context.Context, cr *forgev1.JobTemplate) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if cr.Status.ForgeID > 0 {
		if err := r.Forge.DeleteJobTemplate(ctx, cr.Status.ForgeID); err != nil && !forgeapi.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete forge JobTemplate %d: %w", cr.Status.ForgeID, err)
		}
		logger.Info("deleted JobTemplate from Forge", "id", cr.Status.ForgeID)
	}
	cr.Finalizers = removeString(cr.Finalizers, finalizer)
	return ctrl.Result{}, r.Update(ctx, cr)
}

func (r *JobTemplateReconciler) buildDesired(ctx context.Context, cr *forgev1.JobTemplate) (*forgeapi.JobTemplate, error) {
	name := cr.Spec.Name
	if name == "" {
		name = cr.Name
	}

	invID, err := r.Forge.ResolveInventory(ctx, cr.Spec.Inventory)
	if err != nil {
		return nil, fmt.Errorf("resolve inventory %q: %w", cr.Spec.Inventory, err)
	}
	if invID < 0 {
		return nil, fmt.Errorf("inventory %q not found in Forge", cr.Spec.Inventory)
	}

	projID, err := r.Forge.ResolveProject(ctx, cr.Spec.Project)
	if err != nil {
		return nil, fmt.Errorf("resolve project %q: %w", cr.Spec.Project, err)
	}
	if projID < 0 {
		return nil, fmt.Errorf("project %q not found in Forge", cr.Spec.Project)
	}

	jobType := cr.Spec.JobType
	if jobType == "" {
		jobType = "run"
	}

	return &forgeapi.JobTemplate{
		Name:                  name,
		Description:           cr.Spec.Description,
		JobType:               jobType,
		Inventory:             invID,
		Project:               projID,
		Playbook:              cr.Spec.Playbook,
		Forks:                 cr.Spec.Forks,
		Verbosity:             cr.Spec.Verbosity,
		ExtraVars:             cr.Spec.ExtraVars,
		Limit:                 cr.Spec.Limit,
		AskInventoryOnLaunch:  cr.Spec.AskInventoryOnLaunch,
		AskCredentialOnLaunch: cr.Spec.AskCredentialOnLaunch,
		AskVariablesOnLaunch:  cr.Spec.AskVariablesOnLaunch,
		AskLimitOnLaunch:      cr.Spec.AskLimitOnLaunch,
	}, nil
}

func (r *JobTemplateReconciler) findExisting(ctx context.Context, cr *forgev1.JobTemplate, name string) (*forgeapi.JobTemplate, error) {
	if cr.Status.ForgeID > 0 {
		jt, err := r.Forge.GetJobTemplate(ctx, cr.Status.ForgeID)
		if err == nil {
			return jt, nil
		}
		if !forgeapi.IsNotFound(err) {
			return nil, err
		}
		// fall through to name lookup if the ID is gone (drift / external delete)
	}
	return r.Forge.FindJobTemplateByName(ctx, name)
}

func (r *JobTemplateReconciler) syncCredentials(ctx context.Context, cr *forgev1.JobTemplate, jobTemplateID int64) error {
	desired := map[int64]struct{}{}
	for _, name := range cr.Spec.Credentials {
		id, err := r.Forge.ResolveCredential(ctx, name)
		if err != nil {
			return fmt.Errorf("resolve credential %q: %w", name, err)
		}
		if id < 0 {
			return fmt.Errorf("credential %q not found in Forge", name)
		}
		desired[id] = struct{}{}
	}

	currentIDs, err := r.Forge.ListJobTemplateCredentials(ctx, jobTemplateID)
	if err != nil {
		return err
	}
	current := map[int64]struct{}{}
	for _, id := range currentIDs {
		current[id] = struct{}{}
	}

	// Add what's missing.
	for id := range desired {
		if _, ok := current[id]; !ok {
			if err := r.Forge.AssociateCredential(ctx, jobTemplateID, id); err != nil {
				return fmt.Errorf("associate credential %d: %w", id, err)
			}
		}
	}
	// Remove what's no longer wanted.
	for id := range current {
		if _, ok := desired[id]; !ok {
			if err := r.Forge.DisassociateCredential(ctx, jobTemplateID, id); err != nil {
				return fmt.Errorf("disassociate credential %d: %w", id, err)
			}
		}
	}
	return nil
}

func (r *JobTemplateReconciler) markError(ctx context.Context, cr *forgev1.JobTemplate, reason string, err error) (ctrl.Result, error) {
	setCondition(cr, conditionReady, metav1.ConditionFalse, reason, err.Error())
	setCondition(cr, conditionSynced, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, cr); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager wires the reconciler to JobTemplate events.
func (r *JobTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forgev1.JobTemplate{}).
		Complete(r)
}

// --- helpers ---

func controllerHasFinalizer(cr *forgev1.JobTemplate) bool {
	for _, f := range cr.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	out := slice[:0]
	for _, v := range slice {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

func setCondition(cr *forgev1.JobTemplate, condType string, status metav1.ConditionStatus, reason, msg string) {
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

// equalJobTemplate compares only the fields the operator manages.
func equalJobTemplate(a, b *forgeapi.JobTemplate) bool {
	return a.Name == b.Name &&
		a.Description == b.Description &&
		a.JobType == b.JobType &&
		a.Inventory == b.Inventory &&
		a.Project == b.Project &&
		a.Playbook == b.Playbook &&
		a.Forks == b.Forks &&
		a.Verbosity == b.Verbosity &&
		a.ExtraVars == b.ExtraVars &&
		a.Limit == b.Limit &&
		a.AskInventoryOnLaunch == b.AskInventoryOnLaunch &&
		a.AskCredentialOnLaunch == b.AskCredentialOnLaunch &&
		a.AskVariablesOnLaunch == b.AskVariablesOnLaunch &&
		a.AskLimitOnLaunch == b.AskLimitOnLaunch
}
