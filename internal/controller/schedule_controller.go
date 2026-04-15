package controller

import (
	"bytes"
	"context"
	"encoding/json"
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
	scheduleFinalizer = "schedule.forge.forgeplatform.io/finalizer"
)

// ScheduleReconciler reconciles a Schedule CR with Forge.
//
// Schedules attach to a JobTemplate via unified_job_template. We resolve
// the JobTemplate by name on each reconcile (cheap GET with name filter).
type ScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Forge  *forgeapi.Client
}

// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=schedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=schedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=schedules/finalizers,verbs=update

func (r *ScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr forgev1.Schedule
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ForgeID > 0 {
			if err := r.Forge.DeleteSchedule(ctx, cr.Status.ForgeID); err != nil && !forgeapi.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			logger.Info("deleted Schedule from Forge", "id", cr.Status.ForgeID)
		}
		cr.Finalizers = removeString(cr.Finalizers, scheduleFinalizer)
		return ctrl.Result{}, r.Update(ctx, &cr)
	}

	if !hasFinalizer(cr.Finalizers, scheduleFinalizer) {
		cr.Finalizers = append(cr.Finalizers, scheduleFinalizer)
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Resolve JobTemplate by name (Forge stores schedules by ID).
	jt, err := r.Forge.FindJobTemplateByName(ctx, cr.Spec.JobTemplate)
	if err != nil {
		return r.markScheduleError(ctx, &cr, reasonAPIError, err)
	}
	if jt == nil {
		return r.markScheduleError(ctx, &cr, reasonResolveErr,
			fmt.Errorf("JobTemplate %q not found in Forge", cr.Spec.JobTemplate))
	}

	enabled := true
	if cr.Spec.Enabled != nil {
		enabled = *cr.Spec.Enabled
	}

	desiredName := cr.Spec.Name
	if desiredName == "" {
		desiredName = cr.Name
	}
	// Forge accepts extra_data as a YAML/JSON string. We marshal it
	// as a JSON-encoded string literal so it round-trips cleanly.
	var extraData json.RawMessage
	if cr.Spec.ExtraData != "" {
		raw, err := json.Marshal(cr.Spec.ExtraData)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, fmt.Errorf("encode extraData: %w", err))
		}
		extraData = raw
	}
	desired := &forgeapi.Schedule{
		Name:               desiredName,
		Description:        cr.Spec.Description,
		RRule:              cr.Spec.RRule,
		Enabled:            enabled,
		ExtraData:          extraData,
		UnifiedJobTemplate: jt.ID,
	}

	current := (*forgeapi.Schedule)(nil)
	if cr.Status.ForgeID > 0 {
		current, err = r.Forge.GetSchedule(ctx, cr.Status.ForgeID)
		if err != nil && !forgeapi.IsNotFound(err) {
			return r.markScheduleError(ctx, &cr, reasonAPIError, err)
		}
	}
	if current == nil {
		current, err = r.Forge.FindScheduleByName(ctx, desiredName)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, err)
		}
	}

	switch {
	case current == nil:
		created, err := r.Forge.CreateSchedule(ctx, desired)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, fmt.Errorf("create schedule: %w", err))
		}
		current = created
		logger.Info("created Schedule in Forge", "id", current.ID, "name", current.Name)
	case current.Name != desired.Name || current.Description != desired.Description ||
		current.RRule != desired.RRule || current.Enabled != desired.Enabled ||
		!bytes.Equal(current.ExtraData, desired.ExtraData) || current.UnifiedJobTemplate != desired.UnifiedJobTemplate:
		updated, err := r.Forge.UpdateSchedule(ctx, current.ID, desired)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, fmt.Errorf("update schedule: %w", err))
		}
		current = updated
		logger.Info("updated Schedule in Forge", "id", current.ID)
	}

	cr.Status.ForgeID = current.ID
	cr.Status.JobTemplateID = jt.ID
	cr.Status.NextRun = current.NextRun
	cr.Status.ObservedGeneration = cr.Generation
	setScheduleCondition(&cr, conditionSynced, metav1.ConditionTrue, reasonInSync, "Schedule in sync with Forge")
	setScheduleCondition(&cr, conditionReady, metav1.ConditionTrue, reasonInSync, "")
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *ScheduleReconciler) markScheduleError(ctx context.Context, cr *forgev1.Schedule, reason string, err error) (ctrl.Result, error) {
	setScheduleCondition(cr, conditionReady, metav1.ConditionFalse, reason, err.Error())
	setScheduleCondition(cr, conditionSynced, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, cr); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forgev1.Schedule{}).
		Complete(r)
}

func setScheduleCondition(cr *forgev1.Schedule, condType string, status metav1.ConditionStatus, reason, msg string) {
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
