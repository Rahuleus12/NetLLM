/*
Copyright 2025 AI Provider Platform Authors.

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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

// AIModelSpec defines the desired state of AIModel
type AIModelSpec struct {
	// Name of the model
	Name string `json:"name"`

	// Version of the model
	Version string `json:"version"`

	// Description of the model
	Description string `json:"description,omitempty"`

	// Source configuration for the model
	Source ModelSource `json:"source"`

	// Format of the model (e.g., "pytorch", "tensorflow", "onnx")
	Format string `json:"format"`

	// Size of the model in bytes
	Size int64 `json:"size"`

	// Quantization settings
	Quantization *QuantizationConfig `json:"quantization,omitempty"`

	// Number of replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources requirements
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// GPU configuration
	GPU *GPUConfig `json:"gpu,omitempty"`

	// Auto-scaling configuration
	AutoScaling *AutoScalingConfig `json:"autoScaling,omitempty"`

	// Service configuration
	Service *ServiceConfig `json:"service,omitempty"`

	// Ingress configuration
	Ingress *IngressConfig `json:"ingress,omitempty"`

	// Storage configuration
	Storage *StorageConfig `json:"storage,omitempty"`

	// Health check configuration
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`

	// Environment variables
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Volumes to mount
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Volume mounts
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Node selector
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Priority class name
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// Service account name
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// ModelSource defines the source of the model
type ModelSource struct {
	// Type of source (url, huggingface, s3, pvc)
	Type string `json:"type"`

	// URL to download the model
	URL string `json:"url,omitempty"`

	// HuggingFace model ID
	HuggingFaceModelID string `json:"huggingFaceModelId,omitempty"`

	// HuggingFace revision/branch
	HuggingFaceRevision string `json:"huggingFaceRevision,omitempty"`

	// S3 bucket name
	S3Bucket string `json:"s3Bucket,omitempty"`

	// S3 key/path
	S3Key string `json:"s3Key,omitempty"`

	// PVC name
	PVCName string `json:"pvcName,omitempty"`

	// Path within PVC
	PVCPath string `json:"pvcPath,omitempty"`

	// Authentication configuration
	Auth *AuthConfig `json:"auth,omitempty"`
}

// AuthConfig defines authentication configuration
type AuthConfig struct {
	// Secret name containing auth credentials
	SecretName string `json:"secretName,omitempty"`

	// AWS secret name for S3 access
	AWSSecretName string `json:"awsSecretName,omitempty"`
}

// QuantizationConfig defines quantization settings
type QuantizationConfig struct {
	// Enable quantization
	Enabled bool `json:"enabled"`

	// Type of quantization (int8, int4, fp16, bf16)
	Type string `json:"type"`

	// Calibration dataset
	CalibrationDataset string `json:"calibrationDataset,omitempty"`
}

// GPUConfig defines GPU configuration
type GPUConfig struct {
	// Enable GPU
	Enabled bool `json:"enabled"`

	// Number of GPUs per replica
	Count int32 `json:"count,omitempty"`

	// GPU type (nvidia, amd)
	Type string `json:"type,omitempty"`

	// GPU memory requirement
	Memory string `json:"memory,omitempty"`
}

// AutoScalingConfig defines auto-scaling configuration
type AutoScalingConfig struct {
	// Enable auto-scaling
	Enabled bool `json:"enabled"`

	// Minimum replicas
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// Maximum replicas
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Target CPU utilization percentage
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// Target memory utilization percentage
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`

	// Scale-up cooldown period
	ScaleUpCooldownPeriod *int32 `json:"scaleUpCooldownPeriod,omitempty"`

	// Scale-down cooldown period
	ScaleDownCooldownPeriod *int32 `json:"scaleDownCooldownPeriod,omitempty"`
}

// ServiceConfig defines service configuration
type ServiceConfig struct {
	// Enable service
	Enabled bool `json:"enabled"`

	// Service type
	Type corev1.ServiceType `json:"type,omitempty"`

	// Service port
	Port int32 `json:"port,omitempty"`

	// Target port
	TargetPort int32 `json:"targetPort,omitempty"`

	// Node port (for NodePort type)
	NodePort int32 `json:"nodePort,omitempty"`

	// Annotations
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels
	Labels map[string]string `json:"labels,omitempty"`
}

// IngressConfig defines ingress configuration
type IngressConfig struct {
	// Enable ingress
	Enabled bool `json:"enabled"`

	// Hostname
	Host string `json:"host,omitempty"`

	// Path
	Path string `json:"path,omitempty"`

	// Path type
	PathType networkingv1.PathType `json:"pathType,omitempty"`

	// TLS configuration
	TLS *IngressTLSConfig `json:"tls,omitempty"`

	// Annotations
	Annotations map[string]string `json:"annotations,omitempty"`

	// Ingress class name
	IngressClassName string `json:"ingressClassName,omitempty"`
}

// IngressTLSConfig defines TLS configuration for ingress
type IngressTLSConfig struct {
	// Enable TLS
	Enabled bool `json:"enabled"`

	// Secret name containing TLS certificate
	SecretName string `json:"secretName,omitempty"`
}

// StorageConfig defines storage configuration
type StorageConfig struct {
	// Enable persistent storage
	Enabled bool `json:"enabled"`

	// Storage class name
	StorageClassName string `json:"storageClassName,omitempty"`

	// Storage size
	Size string `json:"size,omitempty"`

	// Access modes
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// HealthCheckConfig defines health check configuration
type HealthCheckConfig struct {
	// Enable health checks
	Enabled bool `json:"enabled"`

	// Liveness probe
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// Readiness probe
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Startup probe
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`
}

// AIModelStatus defines the observed state of AIModel
type AIModelStatus struct {
	// Phase of the model lifecycle
	Phase string `json:"phase,omitempty"`

	// Current state conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Current replicas
	Replicas int32 `json:"replicas,omitempty"`

	// Ready replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Available replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Updated replicas
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// Model loaded status
	ModelLoaded bool `json:"modelLoaded,omitempty"`

	// Endpoint URL
	Endpoint string `json:"endpoint,omitempty"`

	// Inference endpoint
	InferenceEndpoint string `json:"inferenceEndpoint,omitempty"`

	// Metrics endpoint
	MetricsEndpoint string `json:"metricsEndpoint,omitempty"`

	// Health status
	HealthStatus string `json:"healthStatus,omitempty"`

	// Last update time
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Error message
	Error string `json:"error,omitempty"`

	// Warning messages
	Warnings []string `json:"warnings,omitempty"`

	// Performance metrics
	Metrics *ModelMetrics `json:"metrics,omitempty"`
}

// ModelMetrics defines model performance metrics
type ModelMetrics struct {
	// Total inference requests
	TotalRequests int64 `json:"totalRequests,omitempty"`

	// Successful requests
	SuccessfulRequests int64 `json:"successfulRequests,omitempty"`

	// Failed requests
	FailedRequests int64 `json:"failedRequests,omitempty"`

	// Average latency in milliseconds
	AverageLatencyMs float64 `json:"averageLatencyMs,omitempty"`

	// P95 latency in milliseconds
	P95LatencyMs float64 `json:"p95LatencyMs,omitempty"`

	// P99 latency in milliseconds
	P99LatencyMs float64 `json:"p99LatencyMs,omitempty"`

	// Requests per second
	RequestsPerSecond float64 `json:"requestsPerSecond,omitempty"`

	// Average tokens per request
	AvgTokensPerRequest float64 `json:"avgTokensPerRequest,omitempty"`

	// Cache hit rate
	CacheHitRate float64 `json:"cacheHitRate,omitempty"`

	// GPU memory usage percentage
	GPUMemoryUsage float64 `json:"gpuMemoryUsage,omitempty"`

	// GPU utilization percentage
	GPUUtilization float64 `json:"gpuUtilization,omitempty"`
}

// AIModel is the Schema for the aimodels API
type AIModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIModelSpec   `json:"spec,omitempty"`
	Status AIModelStatus `json:"status,omitempty"`
}

// AIModelList contains a list of AIModel
type AIModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIModel `json:"items"`
}

// AIModelReconciler reconciles a AIModel object
type AIModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a AIModel object and makes changes based on the state read
func (r *AIModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.Log.WithValues("aimodel", req.NamespacedName)

	// Fetch the AIModel instance
	aimodel := &AIModel{}
	err := r.Get(ctx, req.NamespacedName, aimodel)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			log.Info("AIModel resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get AIModel")
		return ctrl.Result{}, err
	}

	// Update observed generation
	if aimodel.Status.ObservedGeneration != aimodel.Generation {
		aimodel.Status.ObservedGeneration = aimodel.Generation
		if err := r.Status().Update(ctx, aimodel); err != nil {
			log.Error(err, "Failed to update observed generation")
			return ctrl.Result{}, err
		}
	}

	// Check if the model is being deleted
	if !aimodel.DeletionTimestamp.IsZero() {
		// The object is being deleted
		log.Info("AIModel is being deleted")
		return r.handleDeletion(ctx, aimodel)
	}

	// Add finalizer if not present
	if !containsString(aimodel.Finalizers, "aimodels.ai-provider.io/finalizer") {
		aimodel.Finalizers = append(aimodel.Finalizers, "aimodels.ai-provider.io/finalizer")
		if err := r.Update(ctx, aimodel); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Update status to initializing if not set
	if aimodel.Status.Phase == "" {
		aimodel.Status.Phase = "Initializing"
		if err := r.Status().Update(ctx, aimodel); err != nil {
			log.Error(err, "Failed to update status to Initializing")
			return ctrl.Result{}, err
		}
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, aimodel)
	if err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		return r.updateStatusWithError(ctx, aimodel, err)
	}

	// Reconcile Service
	service, err := r.reconcileService(ctx, aimodel)
	if err != nil {
		log.Error(err, "Failed to reconcile Service")
		return r.updateStatusWithError(ctx, aimodel, err)
	}

	// Reconcile Ingress (if enabled)
	if aimodel.Spec.Ingress != nil && aimodel.Spec.Ingress.Enabled {
		_, err = r.reconcileIngress(ctx, aimodel)
		if err != nil {
			log.Error(err, "Failed to reconcile Ingress")
			return r.updateStatusWithError(ctx, aimodel, err)
		}
	}

	// Reconcile HPA (if auto-scaling is enabled)
	if aimodel.Spec.AutoScaling != nil && aimodel.Spec.AutoScaling.Enabled {
		_, err = r.reconcileHPA(ctx, aimodel)
		if err != nil {
			log.Error(err, "Failed to reconcile HPA")
			return r.updateStatusWithError(ctx, aimodel, err)
		}
	}

	// Update status based on deployment state
	if deployment != nil {
		aimodel.Status.Replicas = deployment.Status.Replicas
		aimodel.Status.ReadyReplicas = deployment.Status.ReadyReplicas
		aimodel.Status.AvailableReplicas = deployment.Status.AvailableReplicas
		aimodel.Status.UpdatedReplicas = deployment.Status.UpdatedReplicas

		// Determine phase based on deployment state
		if deployment.Status.ReadyReplicas == *aimodel.Spec.Replicas {
			aimodel.Status.Phase = "Running"
			aimodel.Status.HealthStatus = "Healthy"
		} else if deployment.Status.ReadyReplicas > 0 {
			aimodel.Status.Phase = "Updating"
			aimodel.Status.HealthStatus = "Degraded"
		} else {
			aimodel.Status.Phase = "Pending"
			aimodel.Status.HealthStatus = "Unhealthy"
		}
	}

	// Update endpoint information
	if service != nil {
		aimodel.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, aimodel.Spec.Service.Port)
		aimodel.Status.InferenceEndpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/v1/completions", service.Name, service.Namespace, aimodel.Spec.Service.Port)
		aimodel.Status.MetricsEndpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/metrics", service.Name, service.Namespace, aimodel.Spec.Service.Port)
	}

	// Update last update time
	aimodel.Status.LastUpdateTime = metav1.Now()

	// Clear any previous errors
	aimodel.Status.Error = ""

	// Update status
	if err := r.Status().Update(ctx, aimodel); err != nil {
		log.Error(err, "Failed to update AIModel status")
		return ctrl.Result{}, err
	}

	// Requeue after 30 seconds for periodic reconciliation
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// reconcileDeployment creates or updates the deployment for the AIModel
func (r *AIModelReconciler) reconcileDeployment(ctx context.Context, aimodel *AIModel) (*appsv1.Deployment, error) {
	log := ctrl.Log.WithValues("aimodel", types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace})

	// Determine replicas
	replicas := int32(1)
	if aimodel.Spec.Replicas != nil {
		replicas = *aimodel.Spec.Replicas
	}

	// Build container
	container := corev1.Container{
		Name:            "model-server",
		Image:           "ai-provider/model-server:latest",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: append(aimodel.Spec.Env,
			corev1.EnvVar{
				Name:  "MODEL_NAME",
				Value: aimodel.Spec.Name,
			},
			corev1.EnvVar{
				Name:  "MODEL_VERSION",
				Value: aimodel.Spec.Version,
			},
			corev1.EnvVar{
				Name:  "MODEL_FORMAT",
				Value: aimodel.Spec.Format,
			},
			corev1.EnvVar{
				Name:  "MODEL_SOURCE_TYPE",
				Value: aimodel.Spec.Source.Type,
			},
			corev1.EnvVar{
				Name:  "MODEL_SOURCE_URL",
				Value: aimodel.Spec.Source.URL,
			},
		),
		Resources: aimodel.Spec.Resources,
		VolumeMounts: append(aimodel.Spec.VolumeMounts,
			corev1.VolumeMount{
				Name:      "model-storage",
				MountPath: "/models",
			},
		),
	}

	// Add GPU resources if enabled
	if aimodel.Spec.GPU != nil && aimodel.Spec.GPU.Enabled {
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(corev1.ResourceList)
		}
		if aimodel.Spec.GPU.Type == "nvidia" {
			container.Resources.Limits["nvidia.com/gpu"] = resource.MustParse(fmt.Sprintf("%d", aimodel.Spec.GPU.Count))
		}
	}

	// Add health checks if enabled
	if aimodel.Spec.HealthCheck != nil && aimodel.Spec.HealthCheck.Enabled {
		if aimodel.Spec.HealthCheck.LivenessProbe != nil {
			container.LivenessProbe = aimodel.Spec.HealthCheck.LivenessProbe
		} else {
			container.LivenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/health",
						Port: intstr.FromInt(8080),
					},
				},
				InitialDelaySeconds: 30,
				TimeoutSeconds:      10,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
		}

		if aimodel.Spec.HealthCheck.ReadinessProbe != nil {
			container.ReadinessProbe = aimodel.Spec.HealthCheck.ReadinessProbe
		} else {
			container.ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/ready",
						Port: intstr.FromInt(8080),
					},
				},
				InitialDelaySeconds: 10,
				TimeoutSeconds:      5,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
		}
	}

	// Build volumes
	volumes := append(aimodel.Spec.Volumes)

	// Add model storage volume
	if aimodel.Spec.Storage != nil && aimodel.Spec.Storage.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name: "model-storage",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: aimodel.Name + "-storage",
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "model-storage",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Build deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aimodel.Name,
			Namespace: aimodel.Namespace,
			Labels: map[string]string{
				"app":           aimodel.Name,
				"app.kubernetes.io/name":       aimodel.Spec.Name,
				"app.kubernetes.io/version":    aimodel.Spec.Version,
				"app.kubernetes.io/component":  "model-server",
				"app.kubernetes.io/managed-by": "ai-model-operator",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "ai-provider.io/v1",
					Kind:       "AIModel",
					Name:       aimodel.Name,
					UID:        aimodel.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": aimodel.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":           aimodel.Name,
						"app.kubernetes.io/name":       aimodel.Spec.Name,
						"app.kubernetes.io/version":    aimodel.Spec.Version,
						"app.kubernetes.io/component":  "model-server",
						"app.kubernetes.io/managed-by": "ai-model-operator",
					},
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
					NodeSelector:       aimodel.Spec.NodeSelector,
					Tolerations:        aimodel.Spec.Tolerations,
					Affinity:           aimodel.Spec.Affinity,
					PriorityClassName:  aimodel.Spec.PriorityClassName,
					ServiceAccountName: aimodel.Spec.ServiceAccountName,
				},
			},
		},
	}

	// Check if deployment already exists
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		// Create new deployment
		log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		if err := r.Create(ctx, deployment); err != nil {
			return nil, err
		}
		return deployment, nil
	} else if err != nil {
		return nil, err
	}

	// Update existing deployment
	log.Info("Updating existing Deployment", "Deployment.Namespace", existingDeployment.Namespace, "Deployment.Name", existingDeployment.Name)
	existingDeployment.Spec = deployment.Spec
	if err := r.Update(ctx, existingDeployment); err != nil {
		return nil, err
	}

	return existingDeployment, nil
}

// reconcileService creates or updates the service for the AIModel
func (r *AIModelReconciler) reconcileService(ctx context.Context, aimodel *AIModel) (*corev1.Service, error) {
	log := ctrl.Log.WithValues("aimodel", types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace})

	// Determine service type and port
	serviceType := corev1.ServiceTypeClusterIP
	port := int32(8080)
	targetPort := int32(8080)

	if aimodel.Spec.Service != nil {
		if aimodel.Spec.Service.Type != "" {
			serviceType = aimodel.Spec.Service.Type
		}
		if aimodel.Spec.Service.Port != 0 {
			port = aimodel.Spec.Service.Port
		}
		if aimodel.Spec.Service.TargetPort != 0 {
			targetPort = aimodel.Spec.Service.TargetPort
		}
	}

	// Build service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aimodel.Name,
			Namespace: aimodel.Namespace,
			Labels: map[string]string{
				"app":           aimodel.Name,
				"app.kubernetes.io/name":       aimodel.Spec.Name,
				"app.kubernetes.io/version":    aimodel.Spec.Version,
				"app.kubernetes.io/component":  "model-server",
				"app.kubernetes.io/managed-by": "ai-model-operator",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "ai-provider.io/v1",
					Kind:       "AIModel",
					Name:       aimodel.Name,
					UID:        aimodel.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: serviceType,
			Selector: map[string]string{
				"app": aimodel.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt(int(targetPort)),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Check if service already exists
	existingService := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace}, existingService)
	if err != nil && errors.IsNotFound(err) {
		// Create new service
		log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		if err := r.Create(ctx, service); err != nil {
			return nil, err
		}
		return service, nil
	} else if err != nil {
		return nil, err
	}

	// Update existing service
	log.Info("Updating existing Service", "Service.Namespace", existingService.Namespace, "Service.Name", existingService.Name)
	existingService.Spec = service.Spec
	if err := r.Update(ctx, existingService); err != nil {
		return nil, err
	}

	return existingService, nil
}

// reconcileIngress creates or updates the ingress for the AIModel
func (r *AIModelReconciler) reconcileIngress(ctx context.Context, aimodel *AIModel) (*networkingv1.Ingress, error) {
	log := ctrl.Log.WithValues("aimodel", types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace})

	// Build ingress
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aimodel.Name,
			Namespace: aimodel.Namespace,
			Labels: map[string]string{
				"app":           aimodel.Name,
				"app.kubernetes.io/name":       aimodel.Spec.Name,
				"app.kubernetes.io/version":    aimodel.Spec.Version,
				"app.kubernetes.io/component":  "model-server",
				"app.kubernetes.io/managed-by": "ai-model-operator",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "ai-provider.io/v1",
					Kind:       "AIModel",
					Name:       aimodel.Name,
					UID:        aimodel.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &aimodel.Spec.Ingress.IngressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: aimodel.Spec.Ingress.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     aimodel.Spec.Ingress.Path,
									PathType: &aimodel.Spec.Ingress.PathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: aimodel.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Add TLS if enabled
	if aimodel.Spec.Ingress.TLS != nil && aimodel.Spec.Ingress.TLS.Enabled {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{aimodel.Spec.Ingress.Host},
				SecretName: aimodel.Spec.Ingress.TLS.SecretName,
			},
		}
	}

	// Check if ingress already exists
	existingIngress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace}, existingIngress)
	if err != nil && errors.IsNotFound(err) {
		// Create new ingress
		log.Info("Creating a new Ingress", "Ingress.Namespace", ingress.Namespace, "Ingress.Name", ingress.Name)
		if err := r.Create(ctx, ingress); err != nil {
			return nil, err
		}
		return ingress, nil
	} else if err != nil {
		return nil, err
	}

	// Update existing ingress
	log.Info("Updating existing Ingress", "Ingress.Namespace", existingIngress.Namespace, "Ingress.Name", existingIngress.Name)
	existingIngress.Spec = ingress.Spec
	if err := r.Update(ctx, existingIngress); err != nil {
		return nil, err
	}

	return existingIngress, nil
}

// reconcileHPA creates or updates the HPA for the AIModel
func (r *AIModelReconciler) reconcileHPA(ctx context.Context, aimodel *AIModel) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	// HPA implementation would go here
	// For brevity, this is a placeholder
	return nil, nil
}

// handleDeletion handles the deletion of an AIModel
func (r *AIModelReconciler) handleDeletion(ctx context.Context, aimodel *AIModel) (ctrl.Result, error) {
	log := ctrl.Log.WithValues("aimodel", types.NamespacedName{Name: aimodel.Name, Namespace: aimodel.Namespace})

	// Check if finalizer is present
	if !containsString(aimodel.Finalizers, "aimodels.ai-provider.io/finalizer") {
		return ctrl.Result{}, nil
	}

	// Perform cleanup operations here
	log.Info("Performing cleanup for AIModel")

	// Remove finalizer
	aimodel.Finalizers = removeString(aimodel.Finalizers, "aimodels.ai-provider.io/finalizer")
	if err := r.Update(ctx, aimodel); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully removed finalizer")
	return ctrl.Result{}, nil
}

// updateStatusWithError updates the AIModel status with an error message
func (r *AIModelReconciler) updateStatusWithError(ctx context.Context, aimodel *AIModel, err error) (ctrl.Result, error) {
	aimodel.Status.Phase = "Error"
	aimodel.Status.Error = err.Error()
	aimodel.Status.HealthStatus = "Unhealthy"
	aimodel.Status.LastUpdateTime = metav1.Now()

	if updateErr := r.Status().Update(ctx, aimodel); updateErr != nil {
		return ctrl.Result{}, updateErr
	}

	// Requeue after 1 minute on error
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&AIModel{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ai-model-operator.ai-provider.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&AIModelReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AIModel")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// Helper functions
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func boolPtr(b bool) *bool {
	return &b
}

// Placeholder for autoscaling import
type autoscalingv2 struct {
	HorizontalPodAutoscaler interface{}
}
