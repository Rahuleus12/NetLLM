package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIModelSpec defines the desired state of AIModel
type AIModelSpec struct {
	// Model identification
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`

	// Model source configuration
	Source ModelSource `json:"source"`

	// Model format and size
	Format string `json:"format"` // e.g., "pytorch", "tensorflow", "onnx"
	Size   string `json:"size"`   // e.g., "7b", "13b", "70b"

	// Quantization settings
	Quantization *QuantizationConfig `json:"quantization,omitempty"`

	// Deployment configuration
	Replicas *int32 `json:"replicas,omitempty"`

	// Resource requirements
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// GPU configuration
	GPU *GPUConfig `json:"gpu,omitempty"`

	// Auto-scaling configuration
	AutoScaling *AutoScalingConfig `json:"autoScaling,omitempty"`

	// Storage configuration
	Storage *StorageConfig `json:"storage,omitempty"`

	// Inference configuration
	Inference *InferenceConfig `json:"inference,omitempty"`

	// Health check configuration
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`

	// Networking configuration
	Networking *NetworkingConfig `json:"networking,omitempty"`

	// Security configuration
	Security *SecurityConfig `json:"security,omitempty"`

	// Metadata and labels
	Metadata map[string]string `json:"metadata,omitempty"`

	// Priority class name
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// Node selector
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity rules
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Service account name
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Pod annotations
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
}

// ModelSource defines where to load the model from
type ModelSource struct {
	// Source type: "url", "huggingface", "s3", "pvc", "git"
	Type string `json:"type"`

	// URL to download model from
	URL string `json:"url,omitempty"`

	// HuggingFace model configuration
	HuggingFace *HuggingFaceSource `json:"huggingFace,omitempty"`

	// S3 configuration
	S3 *S3Source `json:"s3,omitempty"`

	// PVC configuration
	PVC *PVCSource `json:"pvc,omitempty"`

	// Git repository configuration
	Git *GitSource `json:"git,omitempty"`

	// Authentication for private repositories
	Auth *AuthConfig `json:"auth,omitempty"`
}

// HuggingFaceSource defines HuggingFace model source
type HuggingFaceSource struct {
	ModelID  string `json:"modelId"`
	Revision string `json:"revision,omitempty"`
	Token    string `json:"token,omitempty"`
}

// S3Source defines S3 model source
type S3Source struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	Region    string `json:"region,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

// PVCSource defines PersistentVolumeClaim model source
type PVCSource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	SubPath   string `json:"subPath,omitempty"`
}

// GitSource defines Git repository model source
type GitSource struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Path       string `json:"path,omitempty"`
}

// AuthConfig defines authentication configuration
type AuthConfig struct {
	SecretRef string `json:"secretRef,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Token     string `json:"token,omitempty"`
}

// QuantizationConfig defines model quantization settings
type QuantizationConfig struct {
	Enabled     bool   `json:"enabled"`
	Method      string `json:"method,omitempty"`      // "awq", "gptq", "bnb", "fp8"
	Precision   string `json:"precision,omitempty"`   // "int8", "int4", "fp16", "bf16"
	GroupSize   int    `json:"groupSize,omitempty"`   // Group size for quantization
	ActOrder    bool   `json:"actOrder,omitempty"`    // Activation ordering
	DampPercent int    `json:"dampPercent,omitempty"` // Damping percentage
}

// GPUConfig defines GPU configuration
type GPUConfig struct {
	Enabled       bool   `json:"enabled"`
	Type          string `json:"type,omitempty"`          // "nvidia", "amd"
	Count         int    `json:"count,omitempty"`         // Number of GPUs
	MemoryGB      int    `json:"memoryGB,omitempty"`      // GPU memory in GB
	MigEnabled    bool   `json:"migEnabled,omitempty"`    // Multi-Instance GPU
	MigPartition  string `json:"migPartition,omitempty"`  // MIG partition size
	Shared        bool   `json:"shared,omitempty"`        // Shared GPU
	PartitionSize string `json:"partitionSize,omitempty"` // GPU partition size
}

// AutoScalingConfig defines auto-scaling configuration
type AutoScalingConfig struct {
	Enabled     bool  `json:"enabled"`
	MinReplicas int32 `json:"minReplicas,omitempty"`
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Metrics for scaling
	CPUTargetPercent    *int32 `json:"cpuTargetPercent,omitempty"`
	MemoryTargetPercent *int32 `json:"memoryTargetPercent,omitempty"`
	CustomMetrics       []CustomMetricSpec `json:"customMetrics,omitempty"`

	// Scaling behavior
	ScaleUpCooldownSeconds   int32 `json:"scaleUpCooldownSeconds,omitempty"`
	ScaleDownCooldownSeconds int32 `json:"scaleDownCooldownSeconds,omitempty"`
}

// CustomMetricSpec defines custom metric for scaling
type CustomMetricSpec struct {
	Name      string `json:"name"`
	Target    int32  `json:"target"`
	Namespace string `json:"namespace,omitempty"`
}

// StorageConfig defines storage configuration
type StorageConfig struct {
	// Model cache storage
	CacheSize string `json:"cacheSize,omitempty"`

	// Model storage class
	StorageClass string `json:"storageClass,omitempty"`

	// Persistent volume claim template
	PVC *corev1.PersistentVolumeClaimSpec `json:"pvc,omitempty"`

	// Enable model caching
	EnableCache bool `json:"enableCache,omitempty"`

	// Cache directory
	CacheDir string `json:"cacheDir,omitempty"`
}

// InferenceConfig defines inference configuration
type InferenceConfig struct {
	// Inference engine: "vllm", "tgi", "tensorrt-llm", "ollama"
	Engine string `json:"engine,omitempty"`

	// Maximum sequence length
	MaxSequenceLength int `json:"maxSequenceLength,omitempty"`

	// Maximum batch size
	MaxBatchSize int `json:"maxBatchSize,omitempty"`

	// Tensor parallelism
	TensorParallelism int `json:"tensorParallelism,omitempty"`

	// Pipeline parallelism
	PipelineParallelism int `json:"pipelineParallelism,omitempty"`

	// Maximum tokens per request
	MaxTokensPerRequest int `json:"maxTokensPerRequest,omitempty"`

	// Enable streaming
	EnableStreaming bool `json:"enableStreaming,omitempty"`

	// Enable batching
	EnableBatching bool `json:"enableBatching,omitempty"`

	// Batch timeout in milliseconds
	BatchTimeout int `json:"batchTimeout,omitempty"`

	// Enable prefix caching
	EnablePrefixCache bool `json:"enablePrefixCache,omitempty"`

	// Custom arguments
	Args []string `json:"args,omitempty"`

	// Environment variables
	Env map[string]string `json:"env,omitempty"`
}

// HealthCheckConfig defines health check configuration
type HealthCheckConfig struct {
	// Enable health checks
	Enabled bool `json:"enabled"`

	// Health check endpoint
	Path string `json:"path,omitempty"`

	// Health check port
	Port int `json:"port,omitempty"`

	// Initial delay in seconds
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// Period between checks in seconds
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`

	// Timeout in seconds
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// Success threshold
	SuccessThreshold int32 `json:"successThreshold,omitempty"`

	// Failure threshold
	FailureThreshold int32 `json:"failureThreshold,omitempty"`

	// Readiness probe configuration
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Liveness probe configuration
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
}

// NetworkingConfig defines networking configuration
type NetworkingConfig struct {
	// Service type: ClusterIP, NodePort, LoadBalancer
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// Service port
	Port int32 `json:"port,omitempty"`

	// Target port
	TargetPort int32 `json:"targetPort,omitempty"`

	// Enable ingress
	EnableIngress bool `json:"enableIngress,omitempty"`

	// Ingress configuration
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Annotations for service
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// Enable network policy
	EnableNetworkPolicy bool `json:"enableNetworkPolicy,omitempty"`

	// Allowed sources
	AllowedSources []string `json:"allowedSources,omitempty"`
}

// IngressConfig defines ingress configuration
type IngressConfig struct {
	// Ingress class name
	ClassName string `json:"className,omitempty"`

	// Host name
	Host string `json:"host,omitempty"`

	// Path
	Path string `json:"path,omitempty"`

	// Path type
	PathType string `json:"pathType,omitempty"`

	// TLS configuration
	TLS *IngressTLSConfig `json:"tls,omitempty"`

	// Annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// IngressTLSConfig defines ingress TLS configuration
type IngressTLSConfig struct {
	Enabled     bool   `json:"enabled"`
	SecretName  string `json:"secretName,omitempty"`
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"privateKey,omitempty"`
}

// SecurityConfig defines security configuration
type SecurityConfig struct {
	// Run as non-root user
	RunAsNonRoot bool `json:"runAsNonRoot,omitempty"`

	// Run as user
	RunAsUser *int64 `json:"runAsUser,omitempty"`

	// Run as group
	RunAsGroup *int64 `json:"runAsGroup,omitempty"`

	// File system group
	FSGroup *int64 `json:"fsGroup,omitempty"`

	// Read-only root filesystem
	ReadOnlyRootFilesystem bool `json:"readOnlyRootFilesystem,omitempty"`

	// Security context capabilities
	Capabilities *corev1.Capabilities `json:"capabilities,omitempty"`

	// Enable service mesh
	EnableServiceMesh bool `json:"enableServiceMesh,omitempty"`

	// Network policy
	NetworkPolicy *NetworkPolicyConfig `json:"networkPolicy,omitempty"`

	// Pod security policy
	PodSecurityPolicyName string `json:"podSecurityPolicyName,omitempty"`
}

// NetworkPolicyConfig defines network policy configuration
type NetworkPolicyConfig struct {
	// Policy types
	PolicyTypes []string `json:"policyTypes,omitempty"`

	// Ingress rules
	Ingress []NetworkPolicyIngressRule `json:"ingress,omitempty"`

	// Egress rules
	Egress []NetworkPolicyEgressRule `json:"egress,omitempty"`
}

// NetworkPolicyIngressRule defines ingress rule
type NetworkPolicyIngressRule struct {
	Ports    []corev1.NetworkPolicyPort `json:"ports,omitempty"`
	From     []corev1.NetworkPolicyPeer `json:"from,omitempty"`
}

// NetworkPolicyEgressRule defines egress rule
type NetworkPolicyEgressRule struct {
	Ports []corev1.NetworkPolicyPort `json:"ports,omitempty"`
	To    []corev1.NetworkPolicyPeer `json:"to,omitempty"`
}

// AIModelStatus defines the observed state of AIModel
type AIModelStatus struct {
	// Current phase
	Phase ModelPhase `json:"phase"`

	// Current conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready replicas
	ReadyReplicas int32 `json:"readyReplicas"`

	// Total replicas
	TotalReplicas int32 `json:"totalReplicas"`

	// Available replicas
	AvailableReplicas int32 `json:"availableReplicas"`

	// Service endpoint
	Endpoint string `json:"endpoint,omitempty"`

	// Internal endpoint
	InternalEndpoint string `json:"internalEndpoint,omitempty"`

	// Model loaded
	ModelLoaded bool `json:"modelLoaded"`

	// Model size
	ModelSize string `json:"modelSize,omitempty"`

	// Last update time
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Performance metrics
	Metrics *ModelMetrics `json:"metrics,omitempty"`

	// Error message
	Error string `json:"error,omitempty"`

	// Events
	Events []ModelEvent `json:"events,omitempty"`
}

// ModelPhase defines the phase of the model
type ModelPhase string

const (
	// PhasePending - Model is pending deployment
	PhasePending ModelPhase = "Pending"
	// PhaseInitializing - Model is initializing
	PhaseInitializing ModelPhase = "Initializing"
	// PhaseLoading - Model is being loaded
	PhaseLoading ModelPhase = "Loading"
	// PhaseReady - Model is ready and serving requests
	PhaseReady ModelPhase = "Ready"
	// PhaseUpdating - Model is being updated
	PhaseUpdating ModelPhase = "Updating"
	// PhaseScaling - Model is scaling
	PhaseScaling ModelPhase = "Scaling"
	// PhaseDegraded - Model is in degraded state
	PhaseDegraded ModelPhase = "Degraded"
	// PhaseFailed - Model deployment failed
	PhaseFailed ModelPhase = "Failed"
	// PhaseTerminating - Model is terminating
	PhaseTerminating ModelPhase = "Terminating"
)

// ModelMetrics defines performance metrics
type ModelMetrics struct {
	// Average inference latency in milliseconds
	AvgInferenceLatency float64 `json:"avgInferenceLatency,omitempty"`

	// P50 inference latency
	P50InferenceLatency float64 `json:"p50InferenceLatency,omitempty"`

	// P95 inference latency
	P95InferenceLatency float64 `json:"p95InferenceLatency,omitempty"`

	// P99 inference latency
	P99InferenceLatency float64 `json:"p99InferenceLatency,omitempty"`

	// Requests per second
	RequestsPerSecond float64 `json:"requestsPerSecond,omitempty"`

	// Throughput in tokens per second
	TokensPerSecond float64 `json:"tokensPerSecond,omitempty"`

	// GPU utilization
	GPUUtilization float64 `json:"gpuUtilization,omitempty"`

	// Memory utilization
	MemoryUtilization float64 `json:"memoryUtilization,omitempty"`

	// Queue length
	QueueLength int `json:"queueLength,omitempty"`

	// Cache hit rate
	CacheHitRate float64 `json:"cacheHitRate,omitempty"`
}

// ModelEvent defines a model event
type ModelEvent struct {
	// Event time
	Time metav1.Time `json:"time"`

	// Event type
	Type string `json:"type"`

	// Event message
	Message string `json:"message"`

	// Event reason
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIModel is the Schema for the aimodels API
type AIModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIModelSpec   `json:"spec,omitempty"`
	Status AIModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIModelList contains a list of AIModel
type AIModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIModel{}, &AIModelList{})
}

// IsReady returns true if the model is ready
func (m *AIModel) IsReady() bool {
	return m.Status.Phase == PhaseReady && m.Status.ModelLoaded
}

// GetServiceName returns the service name for the model
func (m *AIModel) GetServiceName() string {
	return m.Name + "-service"
}

// GetDeploymentName returns the deployment name for the model
func (m *AIModel) GetDeploymentName() string {
	return m.Name + "-deployment"
}

// GetHPAName returns the HPA name for the model
func (m *AIModel) GetHPAName() string {
	return m.Name + "-hpa"
}

// GetIngressName returns the ingress name for the model
func (m *AIModel) GetIngressName() string {
	return m.Name + "-ingress"
}

// GetLabels returns common labels for the model
func (m *AIModel) GetLabels() map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       m.Name,
		"app.kubernetes.io/instance":   m.Name,
		"app.kubernetes.io/version":    m.Spec.Version,
		"app.kubernetes.io/component":  "ai-model",
		"app.kubernetes.io/part-of":    "ai-provider",
		"app.kubernetes.io/managed-by": "ai-model-operator",
	}

	// Add custom labels
	for k, v := range m.Spec.Metadata {
		labels[k] = v
	}

	return labels
}
