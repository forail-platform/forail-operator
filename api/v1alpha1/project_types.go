package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectSpec mirrors a subset of Forail `/api/v2/projects/`.
// SCM credential (if any) is referenced by name and resolved at reconcile.
type ProjectSpec struct {
	// Display name in Forail. Defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Forail organization name. Required.
	// +kubebuilder:validation:Required
	Organization string `json:"organization"`

	// SCM type. "manual" means a pre-populated project directory on disk.
	// +kubebuilder:validation:Enum=git;hg;svn;insights;archive;manual
	// +kubebuilder:default=git
	ScmType string `json:"scmType,omitempty"`

	// Repository URL. Required for non-manual SCM types.
	// +optional
	ScmURL string `json:"scmUrl,omitempty"`

	// Branch / revision / tag.
	// +optional
	ScmBranch string `json:"scmBranch,omitempty"`

	// Refspec (git only). Useful for fetching PR refs.
	// +optional
	ScmRefspec string `json:"scmRefspec,omitempty"`

	// SCM credential name (looked up in Forail). Empty = public/no auth.
	// +optional
	ScmCredential string `json:"scmCredential,omitempty"`

	// +optional
	ScmClean bool `json:"scmClean,omitempty"`

	// +optional
	ScmDeleteOnUpdate bool `json:"scmDeleteOnUpdate,omitempty"`

	// +optional
	ScmUpdateOnLaunch bool `json:"scmUpdateOnLaunch,omitempty"`

	// SCM update cache TTL in seconds (Forail default 0 = always update).
	// +kubebuilder:validation:Minimum=0
	// +optional
	ScmUpdateCacheTimeout int32 `json:"scmUpdateCacheTimeout,omitempty"`

	// +optional
	AllowOverride bool `json:"allowOverride,omitempty"`

	// Timeout in seconds for SCM update (0 = Forail default).
	// +kubebuilder:validation:Minimum=0
	// +optional
	Timeout int32 `json:"timeout,omitempty"`

	// Default Execution Environment name. Empty = global default.
	// +optional
	DefaultEnvironment string `json:"defaultEnvironment,omitempty"`

	// Optional ForailInstance reference (for multi-cluster). Empty = default.
	// +optional
	ForailInstance string `json:"forailInstance,omitempty"`
}

// ProjectStatus reflects observed state in Forail.
type ProjectStatus struct {
	// +optional
	ForailID int64 `json:"forailId,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Last SCM update status reported by Forail (successful / failed / never).
	// +optional
	ScmRevision string `json:"scmRevision,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=prj,categories=forail
// +kubebuilder:printcolumn:name="Forail ID",type=integer,JSONPath=`.status.forailId`
// +kubebuilder:printcolumn:name="SCM",type=string,JSONPath=`.spec.scmType`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.scmUrl`,priority=1
// +kubebuilder:printcolumn:name="Branch",type=string,JSONPath=`.spec.scmBranch`,priority=1
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Project is the Schema for the projects API.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
