package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	forgev1 "github.com/forgeplatform/forge-operator/api/v1alpha1"
	"github.com/forgeplatform/forge-operator/internal/forgeapi"
)

const (
	credentialFinalizer = "credential.forge.forgeplatform.io/finalizer"
)

// CredentialReconciler reconciles a Credential CR with Forge.
//
// Sensitive fields (passwords, ssh_key_data, vault_token, etc.) come from
// k8s Secrets via spec.inputsFrom. Non-sensitive fields (username,
// become_method) inline in spec.inputs. The operator merges them at
// reconcile time and PATCHes Forge.
type CredentialReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Forge  *forgeapi.Client
}

// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=credentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=credentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=forge.forgeplatform.io,resources=credentials/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *CredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr forgev1.Credential
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ForgeID > 0 {
			if err := r.Forge.DeleteCredential(ctx, cr.Status.ForgeID); err != nil && !forgeapi.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			logger.Info("deleted Credential from Forge", "id", cr.Status.ForgeID)
		}
		cr.Finalizers = removeString(cr.Finalizers, credentialFinalizer)
		return ctrl.Result{}, r.Update(ctx, &cr)
	}

	if !hasFinalizer(cr.Finalizers, credentialFinalizer) {
		cr.Finalizers = append(cr.Finalizers, credentialFinalizer)
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Resolve organization.
	orgID, err := r.Forge.ResolveOrganization(ctx, cr.Spec.Organization)
	if err != nil {
		return r.markCredentialError(ctx, &cr, reasonResolveErr, err)
	}
	if orgID < 0 {
		return r.markCredentialError(ctx, &cr, reasonResolveErr, fmt.Errorf("organization %q not found", cr.Spec.Organization))
	}

	// Resolve credentialType (cached in status).
	ctID := cr.Status.CredentialTypeID
	if ctID == 0 {
		ctID, err = r.Forge.ResolveCredentialType(ctx, cr.Spec.CredentialType)
		if err != nil {
			return r.markCredentialError(ctx, &cr, reasonResolveErr, err)
		}
		if ctID < 0 {
			return r.markCredentialError(ctx, &cr, reasonResolveErr, fmt.Errorf("credential_type %q not found", cr.Spec.CredentialType))
		}
	}

	// Build inputs map: non-sensitive (spec.inputs) + sensitive (Secrets).
	inputs, err := r.assembleInputs(ctx, &cr)
	if err != nil {
		return r.markCredentialError(ctx, &cr, reasonResolveErr, err)
	}
	hash := hashInputs(inputs)

	desiredName := cr.Spec.Name
	if desiredName == "" {
		desiredName = cr.Name
	}
	desired := &forgeapi.Credential{
		Name:           desiredName,
		Description:    cr.Spec.Description,
		Organization:   orgID,
		CredentialType: ctID,
		Inputs:         inputs,
	}

	current := (*forgeapi.Credential)(nil)
	if cr.Status.ForgeID > 0 {
		current, err = r.Forge.GetCredential(ctx, cr.Status.ForgeID)
		if err != nil && !forgeapi.IsNotFound(err) {
			return r.markCredentialError(ctx, &cr, reasonAPIError, err)
		}
	}
	if current == nil {
		current, err = r.Forge.FindCredentialByName(ctx, desiredName)
		if err != nil {
			return r.markCredentialError(ctx, &cr, reasonAPIError, err)
		}
	}

	switch {
	case current == nil:
		created, err := r.Forge.CreateCredential(ctx, desired)
		if err != nil {
			return r.markCredentialError(ctx, &cr, reasonAPIError, fmt.Errorf("create credential: %w", err))
		}
		current = created
		logger.Info("created Credential in Forge", "id", current.ID, "name", current.Name)
	case current.Name != desired.Name || current.Description != desired.Description ||
		current.Organization != desired.Organization || current.CredentialType != desired.CredentialType ||
		hash != cr.Status.SecretsHash:
		updated, err := r.Forge.UpdateCredential(ctx, current.ID, desired)
		if err != nil {
			return r.markCredentialError(ctx, &cr, reasonAPIError, fmt.Errorf("update credential: %w", err))
		}
		current = updated
		logger.Info("updated Credential in Forge", "id", current.ID)
	}

	cr.Status.ForgeID = current.ID
	cr.Status.CredentialTypeID = ctID
	cr.Status.SecretsHash = hash
	cr.Status.ObservedGeneration = cr.Generation
	setCredentialCondition(&cr, conditionSynced, metav1.ConditionTrue, reasonInSync, "Credential in sync with Forge")
	setCredentialCondition(&cr, conditionReady, metav1.ConditionTrue, reasonInSync, "")
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// assembleInputs reads referenced Secrets and merges with spec.inputs.
// Sensitive values from Secrets take precedence on key conflict.
func (r *CredentialReconciler) assembleInputs(ctx context.Context, cr *forgev1.Credential) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range cr.Spec.Inputs {
		out[k] = v
	}
	for _, ref := range cr.Spec.InputsFrom {
		var sec corev1.Secret
		key := types.NamespacedName{Namespace: cr.Namespace, Name: ref.ValueFrom.Name}
		if err := r.Get(ctx, key, &sec); err != nil {
			return nil, fmt.Errorf("read Secret %s/%s: %w", cr.Namespace, ref.ValueFrom.Name, err)
		}
		val, ok := sec.Data[ref.ValueFrom.Key]
		if !ok {
			return nil, fmt.Errorf("Secret %s/%s missing key %q", cr.Namespace, ref.ValueFrom.Name, ref.ValueFrom.Key)
		}
		out[ref.Name] = string(val)
	}
	return out, nil
}

// hashInputs returns a deterministic SHA256 of the inputs map. Used to
// detect Secret rotation without reading current Forge values (which are
// encrypted at rest).
func hashInputs(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(m[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *CredentialReconciler) markCredentialError(ctx context.Context, cr *forgev1.Credential, reason string, err error) (ctrl.Result, error) {
	setCredentialCondition(cr, conditionReady, metav1.ConditionFalse, reason, err.Error())
	setCredentialCondition(cr, conditionSynced, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, cr); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *CredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forgev1.Credential{}).
		// Watch Secrets so a kubectl edit of a referenced Secret triggers
		// reconcile within seconds instead of waiting for the 60s
		// requeue. Without this, rotating an SSH key required an
		// explicit annotation kick on the Credential CR.
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.credentialsReferencingSecret),
		).
		Complete(r)
}

// credentialsReferencingSecret maps a Secret event to all Credentials in
// the same namespace whose spec.inputsFrom references it.
func (r *CredentialReconciler) credentialsReferencingSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	sec, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	var creds forgev1.CredentialList
	if err := r.List(ctx, &creds, client.InNamespace(sec.Namespace)); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for i := range creds.Items {
		cr := &creds.Items[i]
		for _, ref := range cr.Spec.InputsFrom {
			if ref.ValueFrom.Name == sec.Name {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name},
				})
				break
			}
		}
	}
	return reqs
}

func setCredentialCondition(cr *forgev1.Credential, condType string, status metav1.ConditionStatus, reason, msg string) {
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
