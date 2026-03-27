/*
Copyright 2024 AI Provider Authors.

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
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	aiproviderv1 "github.com/ai-provider/api/v1"
)

// AIModelReconciler reconciles a AIModel object
type AIModelReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *AIModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiproviderv1.AIModel{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RecoverPanic:            true,
		}).
		Complete(r)
}

// +kubebuilder:rbac:groups=aiprovider.io,resources=aimodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aiprovider.io,resources=aimodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aiprovider.io,resources=aimodels/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AIModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the AIModel instance
	model := &aiproviderv1.AIModel{}
	if err := r.Get(ctx, req.NamespacedName, model); err != nil {
		if errors.IsNotFound(err) {
			log.Info("AIModel resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AIModel")
		return ctrl.Result{}, err
	}

	// Check if the AIModel instance is marked to be deleted
	if model.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(model, aiproviderv1.AIModelFinalizer) {
			// Run finalization logic for aiproviderFinalizer
			if err := r.finalizeAIModel(ctx, model); err != nil {
				return ctrl.Result{}, err
			}

			// Remove aiproviderFinalizer
			controllerutil.RemoveFinalizer(model, aiproviderv1.AIModelFinalizer)
			if err := r.Update(ctx, model); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(model, aiproviderv1.AIModelFinalizer) {
		controllerutil.AddFinalizer(model, aiproviderv1.AIModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Initialize status if not present
	if model.Status.Phase == "" {
		model.Status.Phase = aiproviderv1.ModelPhasePending
		model.Status.Conditions = []aiproviderv1.ModelCondition{}
		if err := r.Status().Update(ctx, model); err != nil {
			log.Error(err, "Failed to initialize AIModel status")
			return ctrl.Result{}, err
		}
	}

	// Reconcile based on desired phase
	var result ctrl.Result
	var err error

	switch model.Spec.Phase {
	case aiproviderv1.ModelPhaseDeploying:
		result, err = r.reconcileDeployment(ctx, model)
	case aiproviderv1.ModelPhaseScaling:
		result, err = r.reconcileScaling(ctx, model)
	case aiproviderv1.ModelPhaseUpdating:
		result, err = r.reconcileUpdate(ctx, model)
	case aiproviderv1.ModelPhaseUndeploying:
		result, err = r.reconcileUndeployment(ctx, model)
	default:
		// Initial deployment
		result, err = r.reconcileDeployment(ctx, model)
	}

	if err != nil {
		log.Error(err, "Failed to reconcile AIModel")
		r.Recorder.Eventf(model, corev1.EventTypeWarning, "ReconcileError",
			"Failed to reconcile AIModel: %v", err)
		return ctrl.Result{}, err
	}

	return result, nil
}

// reconcileDeployment handles the deployment of an AI model
func (r *AIModelReconciler) reconcileDeployment(ctx context.Context, model *aiproviderv1.AIModel) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling AIModel deployment", "model", model.Name)

	// Update status to deploying
	if model.Status.Phase != aiproviderv1.ModelPhaseDeploying {
		model.Status.Phase = aiproviderv1.ModelPhaseDeploying
		r.setCondition(model, aiproviderv1.ModelConditionTypeProgressing, metav1.ConditionTrue,
			"DeploymentStarted", "AI model deployment started")
		if err := r.Status().Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1. Create or update ConfigMap for model configuration
	if err := r.reconcileConfigMap(ctx, model); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile ConfigMap: %w", err)
	}

	// 2. Create or update Deployment
	if err := r.reconcileDeploymentResource(ctx, model); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile Deployment: %w", err)
	}

	// 3. Create or update Service
	if err := r.reconcileService(ctx, model); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile Service: %w", err)
	}

	// 4. Check deployment status
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Name, Namespace: model.Namespace}, deployment); err != nil {
		if errors.IsNotFound(err) {
			// Deployment not yet created, requeue
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Update status based on deployment state
	if deployment.Status.ReadyReplicas == *model.Spec.Replicas {
		model.Status.Phase = aiproviderv1.ModelPhaseRunning
		model.Status.ReadyReplicas = deployment.Status.ReadyReplicas
		model.Status.AvailableReplicas = deployment.Status.AvailableReplicas
		r.setCondition(model, aiproviderv1.ModelConditionTypeReady, metav1.ConditionTrue,
			"DeploymentReady", "AI model deployment is ready")
		r.setCondition(model, aiproviderv1.ModelConditionTypeAvailable, metav1.ConditionTrue,
			"ModelAvailable", "AI model is available for inference")

		// Update metrics
		model.Status.Metrics = aiproviderv1.ModelMetrics{
			RequestCount:      0,
			AverageLatency:    0,
			ErrorRate:         0,
			LastMetricsUpdate: metav1.Now(),
		}

		r.Recorder.Eventf(model, corev1.EventTypeNormal, "DeploymentReady",
			"AI model %s is ready with %d replicas", model.Name, deployment.Status.ReadyReplicas)
	} else {
		model.Status.ReadyReplicas = deployment.Status.ReadyReplicas
		model.Status.AvailableReplicas = deployment.Status.AvailableReplicas
		r.setCondition(model, aiproviderv1.ModelConditionTypeProgressing, metav1.ConditionTrue,
			"DeploymentProgressing",
			fmt.Sprintf("Deployment progressing: %d/%d replicas ready", deployment.Status.ReadyReplicas, *model.Spec.Replicas))
	}

	// Update endpoint
	model.Status.Endpoints = aiproviderv1.ModelEndpoints{
		HTTP:   fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", model.Name, model.Namespace, model.Spec.Port),
		GRPC:   fmt.Sprintf("%s.%s.svc.cluster.local:%d", model.Name, model.Namespace, model.Spec.Port+1),
		Health: fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/health", model.Name, model.Namespace, model.Spec.Port),
	}

	if err := r.Status().Update(ctx, model); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to continue monitoring
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// reconcileScaling handles scaling of an AI model deployment
func (r *AIModelReconciler) reconcileScaling(ctx context.Context, model *aiproviderv1.AIModel) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling AIModel scaling", "model", model.Name, "replicas", *model.Spec.Replicas)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Name, Namespace: model.Namespace}, deployment); err != nil {
		return ctrl.Result{}, err
	}

	// Update replica count
	if *deployment.Spec.Replicas != *model.Spec.Replicas {
		deployment.Spec.Replicas = model.Spec.Replicas
		if err := r.Update(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(model, corev1.EventTypeNormal, "Scaling",
			"Scaled AI model %s to %d replicas", model.Name, *model.Spec.Replicas)
	}

	// Transition back to running once scaled
	if deployment.Status.ReadyReplicas == *model.Spec.Replicas {
		model.Status.Phase = aiproviderv1.ModelPhaseRunning
		model.Status.ReadyReplicas = deployment.Status.ReadyReplicas
		if err := r.Status().Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// reconcileUpdate handles updates to an AI model
func (r *AIModelReconciler) reconcileUpdate(ctx context.Context, model *aiproviderv1.AIModel) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling AIModel update", "model", model.Name)

	// Update the deployment with new image/tag
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Name, Namespace: model.Namespace}, deployment); err != nil {
		return ctrl.Result{}, err
	}

	// Update container image
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "model" {
			deployment.Spec.Template.Spec.Containers[i].Image = model.Spec.Image
			break
		}
	}

	if err := r.Update(ctx, deployment); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(model, corev1.EventTypeNormal, "Updating",
		"Updating AI model %s to image %s", model.Name, model.Spec.Image)

	// Transition back to running once updated
	model.Status.Phase = aiproviderv1.ModelPhaseDeploying
	if err := r.Status().Update(ctx, model); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// reconcileUndeployment handles undeployment of an AI model
func (r *AIModelReconciler) reconcileUndeployment(ctx context.Context, model *aiproviderv1.AIModel) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling AIModel undeployment", "model", model.Name)

	// Delete deployment
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Name, Namespace: model.Namespace}, deployment); err == nil {
		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Delete service
	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Name, Namespace: model.Namespace}, service); err == nil {
		if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Delete configmap
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Name + "-config", Namespace: model.Namespace}, configMap); err == nil {
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	model.Status.Phase = aiproviderv1.ModelPhaseUndeployed
	if err := r.Status().Update(ctx, model); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileConfigMap creates or updates the ConfigMap for model configuration
func (r *AIModelReconciler) reconcileConfigMap(ctx context.Context, model *aiproviderv1.AIModel) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      model.Name + "-config",
			Namespace: model.Namespace,
		},
		Data: map[string]string{
			"MODEL_NAME":        model.Spec.Name,
			"MODEL_VERSION":     model.Spec.Version,
			"MODEL_PATH":        model.Spec.ModelPath,
			"BATCH_SIZE":        fmt.Sprintf("%d", model.Spec.BatchSize),
			"MAX_SEQUENCE_LEN":  fmt.Sprintf("%d", model.Spec.MaxSequenceLength),
			"PORT":              fmt.Sprintf("%d", model.Spec.Port),
			"ENABLE_GPU":        fmt.Sprintf("%v", model.Spec.Resources.GPU.Enabled),
			"LOG_LEVEL":         "info",
		},
	}

	// Set AIModel instance as the owner
	if err := controllerutil.SetControllerReference(model, configMap, r.Scheme); err != nil {
		return err
	}

	// Check if ConfigMap already exists
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new ConfigMap
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// Update existing ConfigMap
	found.Data = configMap.Data
	return r.Update(ctx, found)
}

// reconcileDeploymentResource creates or updates the Deployment for the model
func (r *AIModelReconciler) reconcileDeploymentResource(ctx context.Context, model *aiproviderv1.AIModel) error {
	replicas := int32(1)
	if model.Spec.Replicas != nil {
		replicas = *model.Spec.Replicas
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      model.Name,
			Namespace: model.Namespace,
			Labels: map[string]string{
				"app":                    model.Name,
				"ai-model":               model.Spec.Name,
				"ai-model-version":       model.Spec.Version,
				"app.kubernetes.io/name": model.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": model.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                    model.Name,
						"ai-model":               model.Spec.Name,
						"ai-model-version":       model.Spec.Version,
						"app.kubernetes.io/name": model.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "model",
							Image:           model.Spec.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: model.Spec.Port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: model.Name + "-config",
										},
									},
								},
							},
							Resources: r.buildResourceRequirements(model),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(int(model.Spec.Port)),
									},
								},
								InitialDelaySeconds: 60,
								TimeoutSeconds:      10,
								PeriodSeconds:       30,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt(int(model.Spec.Port)),
									},
								},
								InitialDelaySeconds: 30,
								TimeoutSeconds:      5,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							VolumeMounts: r.buildVolumeMounts(model),
						},
					},
					Volumes:      r.buildVolumes(model),
					NodeSelector: model.Spec.Scheduling.NodeSelector,
					Tolerations:  model.Spec.Scheduling.Tolerations,
					Affinity:     model.Spec.Scheduling.Affinity,
				},
			},
		},
	}

	// Set AIModel instance as the owner
	if err := controllerutil.SetControllerReference(model, deployment, r.Scheme); err != nil {
		return err
	}

	// Check if Deployment already exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new Deployment
		return r.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	// Update existing Deployment
	found.Spec = deployment.Spec
	return r.Update(ctx, found)
}

// reconcileService creates or updates the Service for the model
func (r *AIModelReconciler) reconcileService(ctx context.Context, model *aiproviderv1.AIModel) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      model.Name,
			Namespace: model.Namespace,
			Labels: map[string]string{
				"app":                    model.Name,
				"app.kubernetes.io/name": model.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": model.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       model.Spec.Port,
					TargetPort: intstr.FromInt(int(model.Spec.Port)),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       model.Spec.Port + 1,
					TargetPort: intstr.FromInt(int(model.Spec.Port + 1)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Set AIModel instance as the owner
	if err := controllerutil.SetControllerReference(model, service, r.Scheme); err != nil {
		return err
	}

	// Check if Service already exists
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create new Service
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	// Update existing Service
	found.Spec.Selector = service.Spec.Selector
	found.Spec.Ports = service.Spec.Ports
	return r.Update(ctx, found)
}

// buildResourceRequirements constructs resource requirements based on model spec
func (r *AIModelReconciler) buildResourceRequirements(model *aiproviderv1.AIModel) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(model.Spec.Resources.CPU.Request),
			corev1.ResourceMemory: resource.MustParse(model.Spec.Resources.Memory.Request),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(model.Spec.Resources.CPU.Limit),
			corev1.ResourceMemory: resource.MustParse(model.Spec.Resources.Memory.Limit),
		},
	}

	// Add GPU resources if enabled
	if model.Spec.Resources.GPU.Enabled {
		resources.Limits[corev1.ResourceName(model.Spec.Resources.GPU.Type)] =
			resource.MustParse(fmt.Sprintf("%d", model.Spec.Resources.GPU.Count))
	}

	return resources
}

// buildVolumeMounts constructs volume mounts based on model spec
func (r *AIModelReconciler) buildVolumeMounts(model *aiproviderv1.AIModel) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "model-storage",
			MountPath: "/models",
			ReadOnly:  true,
		},
		{
			Name:      "cache-storage",
			MountPath: "/cache",
		},
	}

	// Add GPU-specific volume mounts
	if model.Spec.Resources.GPU.Enabled {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "nvidia",
			MountPath: "/usr/local/nvidia",
		})
	}

	return mounts
}

// buildVolumes constructs volumes based on model spec
func (r *AIModelReconciler) buildVolumes(model *aiproviderv1.AIModel) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "model-storage",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: model.Name + "-models",
				},
			},
		},
		{
			Name: "cache-storage",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI), // 10GB
				},
			},
		},
	}

	// Add GPU-specific volumes
	if model.Spec.Resources.GPU.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name: "nvidia",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/nvidia",
				},
			},
		})
	}

	return volumes
}

// setCondition sets a condition on the AIModel status
func (r *AIModelReconciler) setCondition(model *aiproviderv1.AIModel, condType aiproviderv1.ModelConditionType,
	status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Find existing condition
	for i, cond := range model.Status.Conditions {
		if cond.Type == string(condType) {
			if cond.Status != status || cond.Reason != reason || cond.Message != message {
				model.Status.Conditions[i] = aiproviderv1.ModelCondition{
					Type:               condType,
					Status:             status,
					LastUpdateTime:     now,
					LastTransitionTime: now,
					Reason:             reason,
					Message:            message,
				}
			}
			return
		}
	}

	// Add new condition
	model.Status.Conditions = append(model.Status.Conditions, aiproviderv1.ModelCondition{
		Type:               condType,
		Status:             status,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})
}

// finalizeAIModel performs finalization logic for AIModel
func (r *AIModelReconciler) finalizeAIModel(ctx context.Context, model *aiproviderv1.AIModel) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Finalizing AIModel", "model", model.Name)

	// Clean up any external resources
	// - Release GPU allocations
	// - Clean up model cache
	// - Update external registries
	// - Send metrics/metrics

	r.Recorder.Eventf(model, corev1.EventTypeNormal, "Finalized",
		"AI model %s finalized successfully", model.Name)

	return nil
}
