/*
Copyright 2025 AI Provider Authors.

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

package operators

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aiproviderv1 "github.com/ai-provider/api/v1"
)

const (
	// FinalizerName is the finalizer used for cleanup
	FinalizerName = "ai-provider.io/finalizer"

	// RequeueDelay is the default delay for requeuing reconciliation
	RequeueDelay = 30 * time.Second

	// Label keys
	LabelModelName    = "ai-provider.io/model-name"
	LabelModelVersion = "ai-provider.io/model-version"
	LabelManagedBy    = "ai-provider.io/managed-by"
)

// AIModelReconciler reconciles a AIModel object
type AIModelReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AIModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AIModel instance
	model := &aiproviderv1.AIModel{}
	if err := r.Get(ctx, req.NamespacedName, model); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("AIModel resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AIModel")
		return ctrl.Result{}, err
	}

	// Check if the object is being deleted
	if !model.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, model)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(model, FinalizerName) {
		controllerutil.AddFinalizer(model, FinalizerName)
		if err := r.Update(ctx, model); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to AIModel")
	}

	// Update status to processing if not already
	if model.Status.Phase == "" {
		if err := r.updateStatus(ctx, model, aiproviderv1.PhaseInitializing, "Initializing model deployment"); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Main reconciliation logic
	result, err := r.reconcileModel(ctx, model)
	if err != nil {
		logger.Error(err, "Failed to reconcile AIModel")
		r.Recorder.Eventf(model, corev1.EventTypeWarning, "ReconcileError", "Failed to reconcile: %v", err)
		return ctrl.Result{}, err
	}

	return result, nil
}

// reconcileModel handles the main reconciliation logic
func (r *AIModelReconciler) reconcileModel(ctx context.Context, model *aiproviderv1.AIModel) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Validate the model specification
	if err := r.validateModelSpec(model); err != nil {
		logger.Error(err, "Invalid model specification")
		r.Recorder.Event(model, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		return ctrl.Result{}, r.updateStatus(ctx, model, aiproviderv1.PhaseFailed, fmt.Sprintf("Invalid spec: %v", err))
	}

	// Step 2: Ensure storage (PVC) for model
	pvc, err := r.ensureModelStorage(ctx, model)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure storage: %w", err)
	}

	// Step 3: Download model if needed
	if model.Spec.Source.Type != "" {
		if err := r.downloadModel(ctx, model, pvc); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to download model: %w", err)
		}
	}

	// Step 4: Create or update Deployment
	deployment, err := r.ensureDeployment(ctx, model)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure deployment: %w", err)
	}

	// Step 5: Create or update Service
	service, err := r.ensureService(ctx, model)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure service: %w", err)
	}

	// Step 6: Check deployment status
	if err := r.checkDeploymentStatus(ctx, model, deployment); err != nil {
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
	}

	// Step 7: Update endpoints
	if err := r.updateEndpoints(ctx, model, service); err != nil {
		return ctrl.Result{}, err
	}

	// Step 8: Setup auto-scaling if enabled
	if model.Spec.AutoScaling != nil && model.Spec.AutoScaling.Enabled {
		if err := r.ensureAutoScaling(ctx, model); err != nil {
			logger.Error(err, "Failed to setup auto-scaling")
			r.Recorder.Eventf(model, corev1.EventTypeWarning, "AutoScalingError", "Failed to setup: %v", err)
		}
	}

	// Update status to ready
	if model.Status.Phase != aiproviderv1.PhaseReady {
		message := fmt.Sprintf("Model %s v%s is ready", model.Spec.Name, model.Spec.Version)
		if err := r.updateStatus(ctx, model, aiproviderv1.PhaseReady, message); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(model, corev1.EventTypeNormal, "Ready", message)
		logger.Info("AIModel is ready", "name", model.Name, "version", model.Spec.Version)
	}

	// Requeue periodically for health checks
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// handleDeletion handles the deletion of an AIModel
func (r *AIModelReconciler) handleDeletion(ctx context.Context, model *aiproviderv1.AIModel) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(model, FinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Deleting AIModel", "name", model.Name)

	// Cleanup resources
	if err := r.cleanupResources(ctx, model); err != nil {
		logger.Error(err, "Failed to cleanup resources")
		return ctrl.Result{}, err
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(model, FinalizerName)
	if err := r.Update(ctx, model); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully deleted AIModel")
	r.Recorder.Event(model, corev1.EventTypeNormal, "Deleted", "Model deleted successfully")

	return ctrl.Result{}, nil
}

// validateModelSpec validates the model specification
func (r *AIModelReconciler) validateModelSpec(model *aiproviderv1.AIModel) error {
	if model.Spec.Name == "" {
		return fmt.Errorf("model name is required")
	}

	if model.Spec.Version == "" {
		return fmt.Errorf("model version is required")
	}

	if model.Spec.Replicas < 0 {
		return fmt.Errorf("replicas cannot be negative")
	}

	if model.Spec.Resources != nil {
		if model.Spec.Resources.Requests.CPU == "" && model.Spec.Resources.Limits.CPU == "" {
			return fmt.Errorf("CPU resources must be specified")
		}
	}

	return nil
}

// ensureModelStorage creates or updates the PVC for model storage
func (r *AIModelReconciler) ensureModelStorage(ctx context.Context, model *aiproviderv1.AIModel) (*corev1.PersistentVolumeClaim, error) {
	pvcName := fmt.Sprintf("%s-model-storage", model.Name)
	pvc := &corev1.PersistentVolumeClaim{}

	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: model.Namespace}, pvc)
	if err == nil {
		return pvc, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	// Create new PVC
	storageSize := "10Gi"
	if model.Spec.Storage != nil && model.Spec.Storage.Size != "" {
		storageSize = model.Spec.Storage.Size
	}

	storageClass := "fast-ssd"
	if model.Spec.Storage != nil && model.Spec.Storage.StorageClass != "" {
		storageClass = model.Spec.Storage.StorageClass
	}

	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: model.Namespace,
			Labels: map[string]string{
				LabelModelName:    model.Spec.Name,
				LabelModelVersion: model.Spec.Version,
				LabelManagedBy:    "ai-provider",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: model.APIVersion,
					Kind:       model.Kind,
					Name:       model.Name,
					UID:        model.UID,
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
			StorageClassName: &storageClass,
		},
	}

	if err := r.Create(ctx, pvc); err != nil {
		return nil, fmt.Errorf("failed to create PVC: %w", err)
	}

	log.FromContext(ctx).Info("Created PVC for model", "name", pvcName)
	r.Recorder.Eventf(model, corev1.EventTypeNormal, "StorageCreated", "Created PVC %s", pvcName)

	return pvc, nil
}

// downloadModel handles model download from various sources
func (r *AIModelReconciler) ensureModelStorage(ctx context.Context, model *aiproviderv1.AIModel) (*corev1.PersistentVolumeClaim, error) {
	pvcName := fmt.Sprintf("%s-model-storage", model.Name)
	pvc := &corev1.PersistentVolumeClaim{}

	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: model.Namespace}, pvc)
	if err == nil {
		return pvc, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	// Create new PVC
	storageSize := "10Gi"
	if model.Spec.Storage != nil && model.Spec.Storage.Size != "" {
		storageSize = model.Spec.Storage.Size
	}

	storageClass := "fast-ssd"
	if model.Spec.Storage != nil && model.Spec.Storage.StorageClass != "" {
		storageClass = model.Spec.Storage.StorageClass
	}

	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: model.Namespace,
			Labels: map[string]string{
				LabelModelName:    model.Spec.Name,
				LabelModelVersion: model.Spec.Version,
				LabelManagedBy:    "ai-provider",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: model.APIVersion,
					Kind:       model.Kind,
					Name:       model.Name,
					UID:        model.UID,
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
			StorageClassName: &storageClass,
		},
	}

	if err := r.Create(ctx, pvc); err != nil {
		return nil, fmt.Errorf("failed to create PVC: %w", err)
	}

	log.FromContext(ctx).Info("Created PVC for model", "name", pvcName)
	r.Recorder.Eventf(model, corev1.EventTypeNormal, "StorageCreated", "Created PVC %s", pvcName)

	return pvc, nil
}

// ensureDeployment creates or updates the deployment for the model
func (r *AIModelReconciler) ensureDeployment(ctx context.Context, model *aiproviderv1.AIModel) (*appsv1.Deployment, error) {
	deploymentName := fmt.Sprintf("%s-model", model.Name)
	deployment := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: model.Namespace}, deployment)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	desiredDeployment := r.buildDeployment(model, deploymentName)

	if errors.IsNotFound(err) {
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return nil, fmt.Errorf("failed to create deployment: %w", err)
		}
		log.FromContext(ctx).Info("Created deployment", "name", deploymentName)
		r.Recorder.Eventf(model, corev1.EventTypeNormal, "DeploymentCreated", "Created deployment %s", deploymentName)
		return desiredDeployment, nil
	}

	// Update existing deployment if needed
	if !reflect.DeepEqual(deployment.Spec, desiredDeployment.Spec) {
		deployment.Spec = desiredDeployment.Spec
		if err := r.Update(ctx, deployment); err != nil {
			return nil, fmt.Errorf("failed to update deployment: %w", err)
		}
		log.FromContext(ctx).Info("Updated deployment", "name", deploymentName)
		r.Recorder.Eventf(model, corev1.EventTypeNormal, "DeploymentUpdated", "Updated deployment %s", deploymentName)
	}

	return deployment, nil
}

// buildDeployment constructs the deployment for the model
func (r *AIModelReconciler) buildDeployment(model *aiproviderv1.AIModel, name string) *appsv1.Deployment {
	replicas := int32(1)
	if model.Spec.Replicas > 0 {
		replicas = int32(model.Spec.Replicas)
	}

	labels := map[string]string{
		"app":             name,
		LabelModelName:    model.Spec.Name,
		LabelModelVersion: model.Spec.Version,
		LabelManagedBy:    "ai-provider",
	}

	// Add custom labels
	for k, v := range model.Labels {
		labels[k] = v
	}

	// Build container
	container := corev1.Container{
		Name:            "model-server",
		Image:           model.Spec.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "model-storage",
				MountPath: "/models",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "MODEL_NAME",
				Value: model.Spec.Name,
			},
			{
				Name:  "MODEL_VERSION",
				Value: model.Spec.Version,
			},
			{
				Name:  "MODEL_PATH",
				Value: "/models",
			},
		},
	}

	// Add resource requirements
	if model.Spec.Resources != nil {
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(model.Spec.Resources.Requests.CPU),
				corev1.ResourceMemory: resource.MustParse(model.Spec.Resources.Requests.Memory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(model.Spec.Resources.Limits.CPU),
				corev1.ResourceMemory: resource.MustParse(model.Spec.Resources.Limits.Memory),
			},
		}

		if model.Spec.Resources.Requests.GPU > 0 {
			container.Resources.Requests[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse(fmt.Sprintf("%d", model.Spec.Resources.Requests.GPU))
		}
		if model.Spec.Resources.Limits.GPU > 0 {
			container.Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse(fmt.Sprintf("%d", model.Spec.Resources.Limits.GPU))
		}
	}

	// Add environment variables from spec
	for _, env := range model.Spec.Env {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	// Add health checks
	if model.Spec.HealthCheck != nil && model.Spec.HealthCheck.Enabled {
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: model.Spec.HealthCheck.Path,
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 30,
			TimeoutSeconds:      10,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		}
		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: model.Spec.HealthCheck.Path,
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

	// Build pod spec
	podSpec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		RestartPolicy: corev1.RestartPolicyAlways,
		Volumes: []corev1.Volume{
			{
				Name: "model-storage",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-model-storage", model.Name),
					},
				},
			},
		},
	}

	// Add node selector
	if model.Spec.NodeSelector != nil {
		podSpec.NodeSelector = model.Spec.NodeSelector
	}

	// Add tolerations for GPU nodes if needed
	if model.Spec.Resources != nil && (model.Spec.Resources.Requests.GPU > 0 || model.Spec.Resources.Limits.GPU > 0) {
		podSpec.Tolerations = []corev1.Toleration{
			{
				Key:      "nvidia.com/gpu",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		}
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: model.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: model.APIVersion,
					Kind:       model.Kind,
					Name:       model.Name,
					UID:        model.UID,
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
		},
	}
}

// ensureService creates or updates the service for the model
func (r *AIModelReconciler) ensureService(ctx context.Context, model *aiproviderv1.AIModel) (*corev1.Service, error) {
	serviceName := fmt.Sprintf("%s-model", model.Name)
	service := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: model.Namespace}, service)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: model.Namespace,
			Labels: map[string]string{
				LabelModelName:    model.Spec.Name,
				LabelModelVersion: model.Spec.Version,
				LabelManagedBy:    "ai-provider",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: model.APIVersion,
					Kind:       model.Kind,
					Name:       model.Name,
					UID:        model.UID,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": serviceName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if errors.IsNotFound(err) {
		if err := r.Create(ctx, desiredService); err != nil {
			return nil, fmt.Errorf("failed to create service: %w", err)
		}
		log.FromContext(ctx).Info("Created service", "name", serviceName)
		return desiredService, nil
	}

	// Update if needed
	if !reflect.DeepEqual(service.Spec, desiredService.Spec) {
		service.Spec = desiredService.Spec
		if err := r.Update(ctx, service); err != nil {
			return nil, fmt.Errorf("failed to update service: %w", err)
		}
		log.FromContext(ctx).Info("Updated service", "name", serviceName)
	}

	return service, nil
}

// checkDeploymentStatus checks the deployment status and updates model status
func (r *AIModelReconciler) checkDeploymentStatus(ctx context.Context, model *aiproviderv1.AIModel, deployment *appsv1.Deployment) error {
	logger := log.FromContext(ctx)

	// Update replicas status
	model.Status.Replicas = int(deployment.Status.ReadyReplicas)

	// Check if deployment is progressing
	if deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
		message := fmt.Sprintf("Deployment progressing: %d/%d replicas ready", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		if model.Status.Phase != aiproviderv1.PhaseDeploying {
			if err := r.updateStatus(ctx, model, aiproviderv1.PhaseDeploying, message); err != nil {
				return err
			}
		}
		return fmt.Errorf("deployment not ready")
	}

	// Check for deployment conditions
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			if condition.Status == corev1.ConditionFalse {
				logger.Info("Deployment is not progressing", "reason", condition.Reason, "message", condition.Message)
				return fmt.Errorf("deployment not progressing: %s", condition.Message)
			}
		}
		if condition.Type == appsv1.DeploymentReplicaFailure {
			if condition.Status == corev1.ConditionTrue {
				logger.Error(fmt.Errorf(condition.Message), "Deployment replica failure")
				return fmt.Errorf("deployment replica failure: %s", condition.Message)
			}
		}
	}

	return nil
}

// updateEndpoints updates the service endpoints in the model status
func (r *AIModelReconciler) updateEndpoints(ctx context.Context, model *aiproviderv1.AIModel, service *corev1.Service) error {
	serviceName := fmt.Sprintf("%s-model", model.Name)

	model.Status.Endpoints = aiproviderv1.Endpoints{
		HTTP:   fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", serviceName, model.Namespace),
		GRPC:   fmt.Sprintf("%s.%s.svc.cluster.local:9090", serviceName, model.Namespace),
		Metrics: fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/metrics", serviceName, model.Namespace),
	}

	return r.Status().Update(ctx, model)
}

// ensureAutoScaling sets up Horizontal Pod Autoscaler if enabled
func (r *AIModelReconciler) ensureAutoScaling(ctx context.Context, model *aiproviderv1.AIModel) error {
	// Implementation for HPA setup would go here
	// This is a placeholder for future implementation
	return nil
}

// updateStatus updates the model status
func (r *AIModelReconciler) updateStatus(ctx context.Context, model *aiproviderv1.AIModel, phase aiproviderv1.Phase, message string) error {
	now := metav1.Now()

	// Set condition based on phase
	conditionType := "Ready"
	conditionStatus := corev1.ConditionFalse
	conditionReason := "Progressing"

	switch phase {
	case aiproviderv1.PhaseReady:
		conditionStatus = corev1.ConditionTrue
		conditionReason = "ModelReady"
	case aiproviderv1.PhaseFailed:
		conditionReason = "ModelFailed"
	}

	condition := metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             conditionReason,
		Message:            message,
	}

	meta.SetStatusCondition(&model.Status.Conditions, condition)

	model.Status.Phase = phase
	model.Status.Message = message
	model.Status.LastUpdateTime = &now

	return r.Status().Update(ctx, model)
}

// cleanupResources cleans up resources when the model is deleted
func (r *AIModelReconciler) cleanupResources(ctx context.Context, model *aiproviderv1.AIModel) error {
	logger := log.FromContext(ctx)

	// Delete deployment
	deploymentName := fmt.Sprintf("%s-model", model.Name)
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: model.Namespace}, deployment); err == nil {
		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete deployment: %w", err)
		}
		logger.Info("Deleted deployment", "name", deploymentName)
	}

	// Delete service
	serviceName := fmt.Sprintf("%s-model", model.Name)
	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: model.Namespace}, service); err == nil {
		if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete service: %w", err)
		}
		logger.Info("Deleted service", "name", serviceName)
	}

	// Delete PVC (optional, based on annotation)
	if model.Annotations["ai-provider.io/retain-storage"] != "true" {
		pvcName := fmt.Sprintf("%s-model-storage", model.Name)
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: model.Namespace}, pvc); err == nil {
			if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete PVC: %w", err)
			}
			logger.Info("Deleted PVC", "name", pvcName)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiproviderv1.AIModel{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

// Helper types (would normally be imported)
type intstr.IntOrString struct {
	Type   intstr.Type
	IntVal int32
	StrVal string
}

type resource.Quantity struct{}

func resource.MustParse(str string) resource.Quantity {
	return resource.Quantity{}
}
