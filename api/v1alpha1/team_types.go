package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TeamSpec mirrors Forail `/api/v2/teams/`. Team membership is a separate
// M2M relation handled by the controller (/teams/{id}/users/).
type TeamSpec struct {
	// Display name in Forail. Defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Owning organization (by name).
	// +kubebuilder:validation:Required
	Organization string `json:"organization"`

	// Members (Forail usernames). The controller adds/removes users on the
	// team via /api/v2/teams/{id}/users/ to converge to this list.
	// +optional
	Users []string `json:"users,omitempty"`

	// Optional ForailInstance reference (for multi-cluster). Empty = default.
	// +optional
	ForailInstance string `json:"forailInstance,omitempty"`
}

type TeamStatus struct {
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
// +kubebuilder:resource:shortName=tm,categories=forail
// +kubebuilder:printcolumn:name="Forail ID",type=integer,JSONPath=`.status.forailId`
// +kubebuilder:printcolumn:name="Organization",type=string,JSONPath=`.spec.organization`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Team is the Schema for the teams API.
type Team struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TeamSpec   `json:"spec,omitempty"`
	Status TeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team.
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Team `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}
