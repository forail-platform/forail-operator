package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobTemplateSpec defines the desired state of a Forge JobTemplate.
//
// Mirrors a subset of the upstream Forge `/api/v2/job_templates/` resource.
// Reference fields (inventory, project, organization) are by name — the
// operator resolves them to numeric IDs via the Forge API at reconcile time.
type JobTemplateSpec struct {
	// Display name in Forge. Defaults to the metadata.name of the CR.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Enum=run;check
	// +kubebuilder:default=run
	JobType string `json:"jobType,omitempty"`

	// Forge organization name. Required if Forge has multi-tenant orgs.
	// +optional
	Organization string `json:"organization,omitempty"`

	// Inventory name (looked up by name in Forge).
	// +kubebuilder:validation:Required
	Inventory string `json:"inventory"`

	// Project name (looked up by name in Forge).
	// +kubebuilder:validation:Required
	Project string `json:"project"`

	// Playbook file path within the project (e.g. site.yml).
	// +kubebuilder:validation:Required
	Playbook string `json:"playbook"`

	// Forks (parallelism). 0 = use Forge default.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	Forks int32 `json:"forks,omitempty"`

	// Verbosity 0..5.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=0
	// +optional
	Verbosity int32 `json:"verbosity,omitempty"`

	// Free-form YAML/JSON extra_vars passed to ansible.
	// +optional
	ExtraVars string `json:"extraVars,omitempty"`

	// Names of credentials to attach (looked up by name).
	// +optional
	Credentials []string `json:"credentials,omitempty"`

	// Limit (host pattern), --limit equivalent.
	// +optional
	Limit string `json:"limit,omitempty"`

	// AskInventoryOnLaunch and friends — prompts the user at launch.
	// +optional
	AskInventoryOnLaunch bool `json:"askInventoryOnLaunch,omitempty"`
	// +optional
	AskCredentialOnLaunch bool `json:"askCredentialOnLaunch,omitempty"`
	// +optional
	AskVariablesOnLaunch bool `json:"askVariablesOnLaunch,omitempty"`
	// +optional
	AskLimitOnLaunch bool `json:"askLimitOnLaunch,omitempty"`
}

// JobTemplateStatus reflects the observed state in Forge.
type JobTemplateStatus struct {
	// Forge JobTemplate numeric ID (assigned on first successful create).
	// +optional
	ForgeID int64 `json:"forgeId,omitempty"`

	// ObservedGeneration tracks the last spec generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions surface Ready and Synced state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=jt;jobtmpl,categories=forge
// +kubebuilder:printcolumn:name="Forge ID",type=integer,JSONPath=`.status.forgeId`
// +kubebuilder:printcolumn:name="Inventory",type=string,JSONPath=`.spec.inventory`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.project`
// +kubebuilder:printcolumn:name="Playbook",type=string,JSONPath=`.spec.playbook`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// JobTemplate is the Schema for the jobtemplates API.
type JobTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobTemplateSpec   `json:"spec,omitempty"`
	Status JobTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// JobTemplateList contains a list of JobTemplate.
type JobTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JobTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&JobTemplate{}, &JobTemplateList{})
}
