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

const (
	inventoryFinalizer = "inventory.forail.forail-platform.io/finalizer"
)

// InventoryReconciler reconciles an Inventory CR with Forail.
type InventoryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Forail  *forailapi.Client
}

// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=inventories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=inventories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=forail.forail-platform.io,resources=inventories/finalizers,verbs=update

func (r *InventoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr forailv1.Inventory
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ForailID > 0 {
			if err := r.Forail.DeleteInventory(ctx, cr.Status.ForailID); err != nil && !forailapi.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			logger.Info("deleted Inventory from Forail", "id", cr.Status.ForailID)
		}
		cr.Finalizers = removeString(cr.Finalizers, inventoryFinalizer)
		return ctrl.Result{}, r.Update(ctx, &cr)
	}

	if !hasFinalizer(cr.Finalizers, inventoryFinalizer) {
		cr.Finalizers = append(cr.Finalizers, inventoryFinalizer)
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Resolve organization.
	orgID, err := r.Forail.ResolveOrganization(ctx, cr.Spec.Organization)
	if err != nil {
		return r.markInventoryError(ctx, &cr, reasonResolveErr, err)
	}
	if orgID < 0 {
		return r.markInventoryError(ctx, &cr, reasonResolveErr, fmt.Errorf("organization %q not found in Forail", cr.Spec.Organization))
	}

	desiredName := cr.Spec.Name
	if desiredName == "" {
		desiredName = cr.Name
	}

	desired := &forailapi.Inventory{
		Name:         desiredName,
		Description:  cr.Spec.Description,
		Organization: orgID,
		Variables:    cr.Spec.Variables,
	}

	// Find or create.
	current := (*forailapi.Inventory)(nil)
	if cr.Status.ForailID > 0 {
		current, err = r.Forail.GetInventory(ctx, cr.Status.ForailID)
		if err != nil && !forailapi.IsNotFound(err) {
			return r.markInventoryError(ctx, &cr, reasonAPIError, err)
		}
	}
	if current == nil {
		current, err = r.Forail.FindInventoryByName(ctx, desiredName)
		if err != nil {
			return r.markInventoryError(ctx, &cr, reasonAPIError, err)
		}
	}

	if current == nil {
		created, err := r.Forail.CreateInventory(ctx, desired)
		if err != nil {
			return r.markInventoryError(ctx, &cr, reasonAPIError, fmt.Errorf("create inventory: %w", err))
		}
		current = created
		logger.Info("created Inventory in Forail", "id", current.ID, "name", current.Name)
	} else if current.Name != desired.Name || current.Description != desired.Description ||
		current.Organization != desired.Organization || current.Variables != desired.Variables {
		updated, err := r.Forail.UpdateInventory(ctx, current.ID, desired)
		if err != nil {
			return r.markInventoryError(ctx, &cr, reasonAPIError, fmt.Errorf("update inventory: %w", err))
		}
		current = updated
		logger.Info("updated Inventory in Forail", "id", current.ID)
	}

	// Sync hosts (idempotent).
	hostIDByName, err := r.syncHosts(ctx, &cr, current.ID)
	if err != nil {
		return r.markInventoryError(ctx, &cr, reasonAPIError, fmt.Errorf("sync hosts: %w", err))
	}

	// Sync groups + group memberships.
	if err := r.syncGroups(ctx, &cr, current.ID, hostIDByName); err != nil {
		return r.markInventoryError(ctx, &cr, reasonAPIError, fmt.Errorf("sync groups: %w", err))
	}

	// Refresh totals from API for status.
	refreshed, err := r.Forail.GetInventory(ctx, current.ID)
	if err == nil {
		current = refreshed
	}

	cr.Status.ForailID = current.ID
	cr.Status.HostCount = current.HostsCount
	cr.Status.GroupCount = current.GroupsCount
	cr.Status.ObservedGeneration = cr.Generation
	setInventoryCondition(&cr, conditionSynced, metav1.ConditionTrue, reasonInSync, "Inventory in sync with Forail")
	setInventoryCondition(&cr, conditionReady, metav1.ConditionTrue, reasonInSync, "")
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// syncHosts ensures Forail has exactly the hosts listed in the spec.
// Returns a name -> id map for downstream group membership wiring.
func (r *InventoryReconciler) syncHosts(ctx context.Context, cr *forailv1.Inventory, invID int64) (map[string]int64, error) {
	currentHosts, err := r.Forail.ListHosts(ctx, invID)
	if err != nil {
		return nil, err
	}
	currentByName := map[string]forailapi.Host{}
	for _, h := range currentHosts {
		currentByName[h.Name] = h
	}

	desiredByName := map[string]forailv1.InventoryHost{}
	for _, h := range cr.Spec.Hosts {
		desiredByName[h.Name] = h
	}

	idByName := map[string]int64{}

	// Create / update.
	for name, dh := range desiredByName {
		desired := &forailapi.Host{
			Name:        dh.Name,
			Description: dh.Description,
			Enabled:     dh.Enabled,
			Variables:   dh.Variables,
		}
		if cur, ok := currentByName[name]; ok {
			if cur.Description != desired.Description || cur.Enabled != desired.Enabled ||
				cur.Variables != desired.Variables {
				updated, err := r.Forail.UpdateHost(ctx, cur.ID, desired)
				if err != nil {
					return nil, fmt.Errorf("update host %q: %w", name, err)
				}
				idByName[name] = updated.ID
			} else {
				idByName[name] = cur.ID
			}
		} else {
			created, err := r.Forail.CreateHost(ctx, invID, desired)
			if err != nil {
				return nil, fmt.Errorf("create host %q: %w", name, err)
			}
			idByName[name] = created.ID
		}
	}

	// Delete extras.
	for name, cur := range currentByName {
		if _, ok := desiredByName[name]; !ok {
			if err := r.Forail.DeleteHost(ctx, cur.ID); err != nil {
				return nil, fmt.Errorf("delete host %q: %w", name, err)
			}
		}
	}

	return idByName, nil
}

func (r *InventoryReconciler) syncGroups(ctx context.Context, cr *forailv1.Inventory, invID int64, hostIDByName map[string]int64) error {
	currentGroups, err := r.Forail.ListGroups(ctx, invID)
	if err != nil {
		return err
	}
	currentByName := map[string]forailapi.Group{}
	for _, g := range currentGroups {
		currentByName[g.Name] = g
	}

	desiredByName := map[string]forailv1.InventoryGroup{}
	for _, g := range cr.Spec.Groups {
		desiredByName[g.Name] = g
	}

	idByName := map[string]int64{}

	// Phase 1: create/update group records (without memberships).
	for name, dg := range desiredByName {
		desired := &forailapi.Group{
			Name:        dg.Name,
			Description: dg.Description,
			Variables:   dg.Variables,
		}
		if cur, ok := currentByName[name]; ok {
			if cur.Description != desired.Description || cur.Variables != desired.Variables {
				updated, err := r.Forail.UpdateGroup(ctx, cur.ID, desired)
				if err != nil {
					return fmt.Errorf("update group %q: %w", name, err)
				}
				idByName[name] = updated.ID
			} else {
				idByName[name] = cur.ID
			}
		} else {
			created, err := r.Forail.CreateGroup(ctx, invID, desired)
			if err != nil {
				return fmt.Errorf("create group %q: %w", name, err)
			}
			idByName[name] = created.ID
		}
	}

	// Phase 2: sync host memberships per group.
	for name, dg := range desiredByName {
		groupID := idByName[name]
		desiredHostIDs := map[int64]struct{}{}
		for _, hostName := range dg.Hosts {
			hid, ok := hostIDByName[hostName]
			if !ok {
				return fmt.Errorf("group %q references undeclared host %q", name, hostName)
			}
			desiredHostIDs[hid] = struct{}{}
		}
		currentHostIDs, err := r.Forail.ListGroupHosts(ctx, groupID)
		if err != nil {
			return fmt.Errorf("list group hosts %q: %w", name, err)
		}
		currentSet := map[int64]struct{}{}
		for _, id := range currentHostIDs {
			currentSet[id] = struct{}{}
		}
		for hid := range desiredHostIDs {
			if _, ok := currentSet[hid]; !ok {
				if err := r.Forail.AssociateHostWithGroup(ctx, groupID, hid); err != nil {
					return fmt.Errorf("associate host %d to group %q: %w", hid, name, err)
				}
			}
		}
		for hid := range currentSet {
			if _, ok := desiredHostIDs[hid]; !ok {
				if err := r.Forail.DisassociateHostFromGroup(ctx, groupID, hid); err != nil {
					return fmt.Errorf("disassociate host %d from group %q: %w", hid, name, err)
				}
			}
		}
	}

	// Phase 3: sync child groups (group-of-groups).
	for name, dg := range desiredByName {
		parentID := idByName[name]
		desiredChildIDs := map[int64]struct{}{}
		for _, childName := range dg.Children {
			cid, ok := idByName[childName]
			if !ok {
				return fmt.Errorf("group %q references undeclared child group %q", name, childName)
			}
			desiredChildIDs[cid] = struct{}{}
		}
		currentChildIDs, err := r.Forail.ListGroupChildren(ctx, parentID)
		if err != nil {
			return fmt.Errorf("list group children %q: %w", name, err)
		}
		currentSet := map[int64]struct{}{}
		for _, id := range currentChildIDs {
			currentSet[id] = struct{}{}
		}
		for cid := range desiredChildIDs {
			if _, ok := currentSet[cid]; !ok {
				if err := r.Forail.AssociateChildGroup(ctx, parentID, cid); err != nil {
					return fmt.Errorf("associate child %d to group %q: %w", cid, name, err)
				}
			}
		}
		for cid := range currentSet {
			if _, ok := desiredChildIDs[cid]; !ok {
				if err := r.Forail.DisassociateChildGroup(ctx, parentID, cid); err != nil {
					return fmt.Errorf("disassociate child %d from group %q: %w", cid, name, err)
				}
			}
		}
	}

	// Phase 4: delete groups that are no longer desired.
	for name, cur := range currentByName {
		if _, ok := desiredByName[name]; !ok {
			if err := r.Forail.DeleteGroup(ctx, cur.ID); err != nil {
				return fmt.Errorf("delete group %q: %w", name, err)
			}
		}
	}

	return nil
}

func (r *InventoryReconciler) markInventoryError(ctx context.Context, cr *forailv1.Inventory, reason string, err error) (ctrl.Result, error) {
	setInventoryCondition(cr, conditionReady, metav1.ConditionFalse, reason, err.Error())
	setInventoryCondition(cr, conditionSynced, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, cr); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager wires the reconciler.
func (r *InventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forailv1.Inventory{}).
		Complete(r)
}

// --- helpers shared with jobtemplate_controller ---

func hasFinalizer(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func setInventoryCondition(cr *forailv1.Inventory, condType string, status metav1.ConditionStatus, reason, msg string) {
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
