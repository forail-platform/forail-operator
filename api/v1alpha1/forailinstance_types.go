package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ForailInstanceSpec describes a Forail backend the operator should sync to.
// Other CRs reference this by name (in the same namespace) via their
// `spec.forailInstance` field. When that field is empty, the operator uses
// the default Forail config supplied via --forail-url / --forail-token flags
// (FORAIL_URL / FORAIL_TOKEN env vars).
type ForailInstanceSpec struct {
	// Base URL of the Forail backend (e.g.
	// "https://forail-web.forail.svc.cluster.local:8013").
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Secret holding the OAuth2 PAT used as `Authorization: Bearer ...`.
	// The token must be under the key specified by tokenKey (default "token").
	// +kubebuilder:validation:Required
	TokenSecretRef corev1.SecretKeySelector `json:"tokenSecretRef"`

	// Optional Host header to send (when reaching Forail via an Ingress
	// that routes by hostname but URL uses a node IP).
	// +optional
	HostHeader string `json:"hostHeader,omitempty"`

	// Skip TLS verification (development only).
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

type ForailInstanceStatus struct {
	// Reachable is true when the last health probe succeeded.
	// +optional
	Reachable bool `json:"reachable,omitempty"`

	// ServerVersion is the Forail release reported by /api/v2/ping/.
	// +optional
	ServerVersion string `json:"serverVersion,omitempty"`

	// LastChecked is the timestamp of the last probe.
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fi;forailinst,categories=forail
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Reachable",type=boolean,JSONPath=`.status.reachable`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.serverVersion`
// +kubebuilder:printcolumn:name="Last Checked",type=date,JSONPath=`.status.lastChecked`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ForailInstance is the Schema for the forailinstances API.
type ForailInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ForailInstanceSpec   `json:"spec,omitempty"`
	Status ForailInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ForailInstanceList contains a list of ForailInstance.
type ForailInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ForailInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ForailInstance{}, &ForailInstanceList{})
}
