package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OrganizationSpec mirrors Forail `/api/v2/organizations/`.
// Organizations are the top-level tenant container — every other resource
// (Project, Inventory, JobTemplate, Team) belongs to exactly one.
type OrganizationSpec struct {
	// Display name in Forail. Defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Hard cap on hosts visible to this org (0 = unlimited).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	MaxHosts int32 `json:"maxHosts,omitempty"`

	// Default Execution Environment name for jobs in this org.
	// +optional
	DefaultEnvironment string `json:"defaultEnvironment,omitempty"`

	// Optional ForailInstance reference (for multi-cluster). Empty = default.
	// +optional
	ForailInstance string `json:"forailInstance,omitempty"`
}

type OrganizationStatus struct {
	// +optional
	ForailID int64 `json:"forailId,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=org,categories=forail
// +kubebuilder:printcolumn:name="Forail ID",type=integer,JSONPath=`.status.forailId`
// +kubebuilder:printcolumn:name="Max Hosts",type=integer,JSONPath=`.spec.maxHosts`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Organization is the Schema for the organizations API.
type Organization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrganizationSpec   `json:"spec,omitempty"`
	Status OrganizationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrganizationList contains a list of Organization.
type OrganizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Organization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Organization{}, &OrganizationList{})
}
