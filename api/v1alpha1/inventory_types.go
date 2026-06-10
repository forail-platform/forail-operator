package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InventoryHost is a single host inside the inventory.
type InventoryHost struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Free-form YAML / JSON host variables.
	// +optional
	Variables string `json:"variables,omitempty"`
}

// InventoryGroup is a group of hosts and/or sub-groups.
type InventoryGroup struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +optional
	Description string `json:"description,omitempty"`

	// Free-form YAML / JSON group variables.
	// +optional
	Variables string `json:"variables,omitempty"`

	// Hosts that belong to this group (must also be declared at top
	// level under spec.hosts).
	// +optional
	Hosts []string `json:"hosts,omitempty"`

	// Child group names (must also be declared at top level under
	// spec.groups). Forms group-of-groups hierarchies.
	// +optional
	Children []string `json:"children,omitempty"`
}

// InventorySpec defines the desired state of a Forail Inventory.
type InventorySpec struct {
	// Display name in Forail. Defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Required
	Organization string `json:"organization"`

	// Free-form inventory-wide variables (YAML/JSON).
	// +optional
	Variables string `json:"variables,omitempty"`

	// Hosts to ensure exist in this inventory. Operator will add new
	// ones, update changed ones, and remove ones that are no longer
	// listed (idempotent reconcile).
	// +optional
	Hosts []InventoryHost `json:"hosts,omitempty"`

	// Groups to ensure exist. Same idempotency semantics as Hosts.
	// +optional
	Groups []InventoryGroup `json:"groups,omitempty"`
}

// InventoryStatus reflects the observed Forail state.
type InventoryStatus struct {
	// +optional
	ForailID int64 `json:"forailId,omitempty"`

	// +optional
	HostCount int32 `json:"hostCount,omitempty"`

	// +optional
	GroupCount int32 `json:"groupCount,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=inv,categories=forail
// +kubebuilder:printcolumn:name="Forail ID",type=integer,JSONPath=`.status.forailId`
// +kubebuilder:printcolumn:name="Org",type=string,JSONPath=`.spec.organization`
// +kubebuilder:printcolumn:name="Hosts",type=integer,JSONPath=`.status.hostCount`
// +kubebuilder:printcolumn:name="Groups",type=integer,JSONPath=`.status.groupCount`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Inventory is the Schema for the inventories API.
type Inventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InventorySpec   `json:"spec,omitempty"`
	Status InventoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InventoryList contains a list of Inventory.
type InventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inventory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Inventory{}, &InventoryList{})
}
