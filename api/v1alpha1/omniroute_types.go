/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OmniRouteSpec defines the desired state of OmniRoute.
type OmniRouteSpec struct {
	// Image configures the OmniRoute container image.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Replicas is the number of OmniRoute pods. Defaults to 1.
	// OmniRoute uses SQLite which only supports single-writer, so >1 is unsafe.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Service configures the Kubernetes Service for OmniRoute.
	// +optional
	Service ServiceSpec `json:"service,omitempty"`

	// Resources defines the compute resource requirements for the OmniRoute container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Config references an existing ConfigMap containing OmniRoute environment variables.
	// All keys in the ConfigMap are injected as environment variables via envFrom.
	// +optional
	Config ConfigSpec `json:"config,omitempty"`

	// Credentials references an existing Secret containing required secrets
	// (JWT_SECRET, API_KEY_SECRET, INITIAL_PASSWORD).
	// +optional
	Credentials CredentialsSpec `json:"credentials,omitempty"`

	// Persistence configures the PersistentVolumeClaim for OmniRoute's SQLite data directory.
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`

	// Probes configures the health check probes for the OmniRoute container.
	// +optional
	Probes *ProbesSpec `json:"probes,omitempty"`

	// Ingress configures an optional Ingress resource for external access.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Redis configures an optional Redis connection for rate limiting.
	// +optional
	Redis *RedisSpec `json:"redis,omitempty"`

	// Encryption configures database encryption at rest.
	// +optional
	Encryption *EncryptionSpec `json:"encryption,omitempty"`

	// BasePath sets the OMNIROUTE_BASE_PATH for reverse proxy deployments (e.g. "/omniroute").
	// +optional
	BasePath string `json:"basePath,omitempty"`

	// Env allows setting additional environment variables on the OmniRoute container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// ImageSpec configures the container image.
type ImageSpec struct {
	// Repository is the container image repository.
	// +kubebuilder:default="diegosouzapw/omniroute"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag.
	// +kubebuilder:default="3.8.51"
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy defines when to pull the image.
	// +kubebuilder:default="IfNotPresent"
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// ServiceSpec configures the Kubernetes Service.
type ServiceSpec struct {
	// Type is the Kubernetes Service type.
	// +kubebuilder:default="ClusterIP"
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`

	// Port is the Service port. Defaults to 20128 (OmniRoute's default).
	// +kubebuilder:default=20128
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Annotations are additional annotations to add to the Service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ConfigSpec references an existing ConfigMap.
type ConfigSpec struct {
	// ExistingConfigMap is the name of an existing ConfigMap with OmniRoute env vars.
	// All keys are injected as environment variables via envFrom.
	// +optional
	ExistingConfigMap string `json:"existingConfigMap,omitempty"`
}

// CredentialsSpec references an existing Secret.
type CredentialsSpec struct {
	// ExistingSecret is the name of an existing Secret containing JWT_SECRET,
	// API_KEY_SECRET, and INITIAL_PASSWORD keys.
	// Additional keys like STORAGE_ENCRYPTION_KEY are also injected if present.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`
}

// PersistenceSpec configures PVC storage for OmniRoute data.
type PersistenceSpec struct {
	// Enabled controls whether a PVC is created for the data directory.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// StorageClass is the name of the StorageClass for the PVC.
	// Empty string uses the cluster default.
	// +optional
	StorageClass *string `json:"storageClass,omitempty"`

	// Size is the requested storage size.
	// +kubebuilder:default="1Gi"
	// +optional
	Size resource.Quantity `json:"size,omitempty"`

	// AccessMode is the PVC access mode.
	// +kubebuilder:default="ReadWriteOnce"
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany;ReadOnlyMany
	// +optional
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

// ProbesSpec configures health check probes.
type ProbesSpec struct {
	// LivenessPath is the HTTP path for the liveness probe.
	// +kubebuilder:default="/api/monitoring/health"
	// +optional
	LivenessPath string `json:"livenessPath,omitempty"`

	// ReadinessPath is the HTTP path for the readiness probe.
	// +kubebuilder:default="/api/monitoring/health"
	// +optional
	ReadinessPath string `json:"readinessPath,omitempty"`

	// StartupPath is the HTTP path for the startup probe.
	// +kubebuilder:default="/api/monitoring/health"
	// +optional
	StartupPath string `json:"startupPath,omitempty"`
}

// IngressSpec configures the Ingress resource.
type IngressSpec struct {
	// Enabled controls whether an Ingress resource is created.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ClassName is the IngressClass to use.
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Host is the hostname for the Ingress rule.
	// +optional
	Host string `json:"host,omitempty"`

	// TLS configures TLS for the Ingress.
	// +optional
	TLS *IngressTLSSpec `json:"tls,omitempty"`

	// Annotations are additional annotations for the Ingress.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// IngressTLSSpec configures TLS for the Ingress.
type IngressTLSSpec struct {
	// SecretName is the name of the TLS Secret.
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// RedisSpec configures Redis for rate limiting.
type RedisSpec struct {
	// URL is the Redis connection URL (e.g. redis://redis:6379).
	// +optional
	URL string `json:"url,omitempty"`

	// ExistingSecret is the name of a Secret containing a REDIS_URL key.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`

	// KeyPrefix is the Redis key namespace prefix.
	// +optional
	KeyPrefix string `json:"keyPrefix,omitempty"`
}

// EncryptionSpec configures database encryption at rest.
type EncryptionSpec struct {
	// ExistingSecret is the name of a Secret containing a STORAGE_ENCRYPTION_KEY key.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`

	// KeyVersion is the encryption key version for key rotation.
	// +optional
	KeyVersion string `json:"keyVersion,omitempty"`
}

// ConditionType represents the type of an OmniRoute condition.
type ConditionType string

const (
	// ConditionReady indicates the OmniRoute instance is fully operational.
	ConditionReady ConditionType = "Ready"

	// ConditionProgressing indicates the OmniRoute instance is being rolled out.
	ConditionProgressing ConditionType = "Progressing"

	// ConditionDegraded indicates the OmniRoute instance is not fully healthy.
	ConditionDegraded ConditionType = "Degraded"
)

// OmniRouteStatus defines the observed state of OmniRoute.
type OmniRouteStatus struct {
	// Conditions represent the latest available observations of the OmniRoute state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ReadyReplicas is the number of pods with ready status.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is a simple high-level summary of the OmniRoute lifecycle.
	// +optional
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image.tag",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OmniRoute is the Schema for the omniroutes API.
type OmniRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OmniRouteSpec   `json:"spec,omitempty"`
	Status OmniRouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OmniRouteList contains a list of OmniRoute.
type OmniRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OmniRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniRoute{}, &OmniRouteList{})
}
