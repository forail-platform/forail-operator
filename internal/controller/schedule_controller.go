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

	forailv1 "github.com/forail-platform/forail-operator/api/v1alpha1"
	"github.com/forail-platform/forail-operator/internal/forailapi"
)

const (
	scheduleFinalizer = "schedule.forail.forail-platform.io/finalizer"
)

// ScheduleReconciler reconciles a Schedule CR with Forail.
//
// Schedules attach to a JobTemplate via unified_job_template. We resolve
// the JobTemplate by name on each reconcile (cheap GET with name filter).
type ScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Forail  *forailapi.Client
}

// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=schedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=schedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=schedules/finalizers,verbs=update

func (r *ScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr forailv1.Schedule
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ForailID > 0 {
			if err := r.Forail.DeleteSchedule(ctx, cr.Status.ForailID); err != nil && !forailapi.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			logger.Info("deleted Schedule from Forail", "id", cr.Status.ForailID)
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

	// Resolve JobTemplate by name (Forail stores schedules by ID).
	jt, err := r.Forail.FindJobTemplateByName(ctx, cr.Spec.JobTemplate)
	if err != nil {
		return r.markScheduleError(ctx, &cr, reasonAPIError, err)
	}
	if jt == nil {
		return r.markScheduleError(ctx, &cr, reasonResolveErr,
			fmt.Errorf("JobTemplate %q not found in Forail", cr.Spec.JobTemplate))
	}

	enabled := true
	if cr.Spec.Enabled != nil {
		enabled = *cr.Spec.Enabled
	}

	desiredName := cr.Spec.Name
	if desiredName == "" {
		desiredName = cr.Name
	}
	// Forail accepts extra_data as a YAML/JSON string. We marshal it
	// as a JSON-encoded string literal so it round-trips cleanly.
	var extraData json.RawMessage
	if cr.Spec.ExtraData != "" {
		raw, err := json.Marshal(cr.Spec.ExtraData)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, fmt.Errorf("encode extraData: %w", err))
		}
		extraData = raw
	}
	desired := &forailapi.Schedule{
		Name:               desiredName,
		Description:        cr.Spec.Description,
		RRule:              cr.Spec.RRule,
		Enabled:            enabled,
		ExtraData:          extraData,
		UnifiedJobTemplate: jt.ID,
	}

	current := (*forailapi.Schedule)(nil)
	if cr.Status.ForailID > 0 {
		current, err = r.Forail.GetSchedule(ctx, cr.Status.ForailID)
		if err != nil && !forailapi.IsNotFound(err) {
			return r.markScheduleError(ctx, &cr, reasonAPIError, err)
		}
	}
	if current == nil {
		current, err = r.Forail.FindScheduleByName(ctx, desiredName)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, err)
		}
	}

	switch {
	case current == nil:
		created, err := r.Forail.CreateSchedule(ctx, desired)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, fmt.Errorf("create schedule: %w", err))
		}
		current = created
		logger.Info("created Schedule in Forail", "id", current.ID, "name", current.Name)
	case current.Name != desired.Name || current.Description != desired.Description ||
		current.RRule != desired.RRule || current.Enabled != desired.Enabled ||
		!bytes.Equal(current.ExtraData, desired.ExtraData) || current.UnifiedJobTemplate != desired.UnifiedJobTemplate:
		updated, err := r.Forail.UpdateSchedule(ctx, current.ID, desired)
		if err != nil {
			return r.markScheduleError(ctx, &cr, reasonAPIError, fmt.Errorf("update schedule: %w", err))
		}
		current = updated
		logger.Info("updated Schedule in Forail", "id", current.ID)
	}

	cr.Status.ForailID = current.ID
	cr.Status.JobTemplateID = jt.ID
	cr.Status.NextRun = current.NextRun
	cr.Status.ObservedGeneration = cr.Generation
	setScheduleCondition(&cr, conditionSynced, metav1.ConditionTrue, reasonInSync, "Schedule in sync with Forail")
	setScheduleCondition(&cr, conditionReady, metav1.ConditionTrue, reasonInSync, "")
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *ScheduleReconciler) markScheduleError(ctx context.Context, cr *forailv1.Schedule, reason string, err error) (ctrl.Result, error) {
	setScheduleCondition(cr, conditionReady, metav1.ConditionFalse, reason, err.Error())
	setScheduleCondition(cr, conditionSynced, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, cr); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forailv1.Schedule{}).
		Complete(r)
}

func setScheduleCondition(cr *forailv1.Schedule, condType string, status metav1.ConditionStatus, reason, msg string) {
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
