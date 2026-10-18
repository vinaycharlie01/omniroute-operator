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

package controller

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1alpha1 "github.com/vinaycharlie01/omniroute-operator/api/v1alpha1"
)

const (
	defaultImage        = "diegosouzapw/omniroute"
	defaultTag          = "3.8.51"
	defaultPort         = int32(20128)
	containerName       = "omniroute"
	dataVolumeName      = "data"
	dataMountPath       = "/app/data"
	finalizerName       = "gateway.omniroute.ai/finalizer"
	defaultHealthPath   = "/api/monitoring/health"
	defaultLivenessPath = "/api/monitoring/health"
	envRedisURL         = "REDIS_URL"
)

// OmniRouteReconciler reconciles a OmniRoute object
type OmniRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gateway.omniroute.ai,resources=omniroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.omniroute.ai,resources=omniroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.omniroute.ai,resources=omniroutes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *OmniRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	omniroute := &gatewayv1alpha1.OmniRoute{}
	if err := r.Get(ctx, req.NamespacedName, omniroute); err != nil {
		if errors.IsNotFound(err) {
			log.Info("OmniRoute resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle finalizer for cleanup.
	if omniroute.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(omniroute, finalizerName) {
			log.Info("Running finalizer cleanup")
			controllerutil.RemoveFinalizer(omniroute, finalizerName)
			if err := r.Update(ctx, omniroute); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(omniroute, finalizerName) {
		controllerutil.AddFinalizer(omniroute, finalizerName)
		if err := r.Update(ctx, omniroute); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set Progressing condition at the start of reconciliation.
	r.setCondition(omniroute, gatewayv1alpha1.ConditionProgressing, metav1.ConditionTrue, "Reconciling", "Reconciliation in progress")

	// Reconcile owned resources.
	persistEnabled := omniroute.Spec.Persistence.Enabled == nil || *omniroute.Spec.Persistence.Enabled
	if persistEnabled {
		if err := r.reconcilePVC(ctx, omniroute); err != nil {
			r.setCondition(omniroute, gatewayv1alpha1.ConditionDegraded, metav1.ConditionTrue, "PVCFailed", err.Error())
			_ = r.statusUpdate(ctx, omniroute)
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDeployment(ctx, omniroute); err != nil {
		r.setCondition(omniroute, gatewayv1alpha1.ConditionDegraded, metav1.ConditionTrue, "DeploymentFailed", err.Error())
		_ = r.statusUpdate(ctx, omniroute)
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, omniroute); err != nil {
		r.setCondition(omniroute, gatewayv1alpha1.ConditionDegraded, metav1.ConditionTrue, "ServiceFailed", err.Error())
		_ = r.statusUpdate(ctx, omniroute)
		return ctrl.Result{}, err
	}

	if err := r.reconcileIngress(ctx, omniroute); err != nil {
		r.setCondition(omniroute, gatewayv1alpha1.ConditionDegraded, metav1.ConditionTrue, "IngressFailed", err.Error())
		_ = r.statusUpdate(ctx, omniroute)
		return ctrl.Result{}, err
	}

	// Update status from the Deployment.
	r.updateStatusFromDeployment(ctx, omniroute)

	if err := r.statusUpdate(ctx, omniroute); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OmniRouteReconciler) reconcilePVC(ctx context.Context, cr *gatewayv1alpha1.OmniRoute) error {
	log := logf.FromContext(ctx)
	desired := r.buildPVC(cr)

	if err := controllerutil.SetControllerReference(cr, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on PVC: %w", err)
	}

	existing := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if errors.IsNotFound(err) {
		log.Info("Creating PVC", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	return nil
}

func (r *OmniRouteReconciler) buildPVC(cr *gatewayv1alpha1.OmniRoute) *corev1.PersistentVolumeClaim {
	labels := labelsForOmniRoute(cr.Name)

	size := cr.Spec.Persistence.Size
	if size.IsZero() {
		size = resource.MustParse("1Gi")
	}

	accessMode := corev1.ReadWriteOnce
	if cr.Spec.Persistence.AccessMode != "" {
		accessMode = cr.Spec.Persistence.AccessMode
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName(cr.Name),
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	if cr.Spec.Persistence.StorageClass != nil {
		pvc.Spec.StorageClassName = cr.Spec.Persistence.StorageClass
	}

	return pvc
}

func (r *OmniRouteReconciler) reconcileDeployment(ctx context.Context, cr *gatewayv1alpha1.OmniRoute) error {
	log := logf.FromContext(ctx)
	desired := r.buildDeployment(cr)

	if err := controllerutil.SetControllerReference(cr, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on Deployment: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if errors.IsNotFound(err) {
		log.Info("Creating Deployment", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec = desired.Spec
	log.Info("Updating Deployment", "name", existing.Name)
	return r.Update(ctx, existing)
}

func (r *OmniRouteReconciler) reconcileService(ctx context.Context, cr *gatewayv1alpha1.OmniRoute) error {
	log := logf.FromContext(ctx)
	desired := r.buildService(cr)

	if err := controllerutil.SetControllerReference(cr, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on Service: %w", err)
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if errors.IsNotFound(err) {
		log.Info("Creating Service", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Type = desired.Spec.Type
	log.Info("Updating Service", "name", existing.Name)
	return r.Update(ctx, existing)
}

func (r *OmniRouteReconciler) reconcileIngress(ctx context.Context, cr *gatewayv1alpha1.OmniRoute) error {
	log := logf.FromContext(ctx)

	if cr.Spec.Ingress == nil || !cr.Spec.Ingress.Enabled {
		// Delete Ingress if it exists and is no longer desired.
		existing := &networkingv1.Ingress{}
		err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, existing)
		if err == nil {
			log.Info("Deleting Ingress (no longer enabled)", "name", existing.Name)
			return r.Delete(ctx, existing)
		}
		if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	desired := r.buildIngress(cr)
	if err := controllerutil.SetControllerReference(cr, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on Ingress: %w", err)
	}

	existing := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if errors.IsNotFound(err) {
		log.Info("Creating Ingress", "name", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec = desired.Spec
	existing.Annotations = desired.Annotations
	log.Info("Updating Ingress", "name", existing.Name)
	return r.Update(ctx, existing)
}

func (r *OmniRouteReconciler) buildIngress(cr *gatewayv1alpha1.OmniRoute) *networkingv1.Ingress {
	labels := labelsForOmniRoute(cr.Name)
	port := servicePort(cr)
	pathType := networkingv1.PathTypePrefix

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Namespace:   cr.Namespace,
			Labels:      labels,
			Annotations: cr.Spec.Ingress.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: cr.Spec.Ingress.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: cr.Spec.Ingress.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: cr.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: port,
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

	if cr.Spec.Ingress.TLS != nil && cr.Spec.Ingress.TLS.SecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{cr.Spec.Ingress.Host},
				SecretName: cr.Spec.Ingress.TLS.SecretName,
			},
		}
	}

	return ingress
}

func (r *OmniRouteReconciler) buildDeployment(cr *gatewayv1alpha1.OmniRoute) *appsv1.Deployment {
	labels := labelsForOmniRoute(cr.Name)
	replicas := int32(1)
	if cr.Spec.Replicas != nil {
		replicas = *cr.Spec.Replicas
	}

	image := imageString(cr)
	port := servicePort(cr)

	livenessPath, readinessPath, startupPath := probePaths(cr)

	container := corev1.Container{
		Name:  containerName,
		Image: image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:     buildEnvVars(cr, port),
		EnvFrom: buildEnvFrom(cr),
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: livenessPath,
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: readinessPath,
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: startupPath,
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
			TimeoutSeconds:      5,
			FailureThreshold:    30,
		},
	}

	if cr.Spec.Image.PullPolicy != "" {
		container.ImagePullPolicy = cr.Spec.Image.PullPolicy
	}

	if cr.Spec.Resources.Requests != nil || cr.Spec.Resources.Limits != nil {
		container.Resources = cr.Spec.Resources
	}

	volumes, volumeMounts := buildVolumes(cr)
	container.VolumeMounts = volumeMounts

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers:                    []corev1.Container{container},
					Volumes:                       volumes,
					TerminationGracePeriodSeconds: int64Ptr(40),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(1000),
						RunAsGroup:   int64Ptr(1000),
						FSGroup:      int64Ptr(1000),
					},
				},
			},
		},
	}

	return dep
}

func (r *OmniRouteReconciler) buildService(cr *gatewayv1alpha1.OmniRoute) *corev1.Service {
	labels := labelsForOmniRoute(cr.Name)
	port := servicePort(cr)

	svcType := corev1.ServiceTypeClusterIP
	if cr.Spec.Service.Type != "" {
		svcType = cr.Spec.Service.Type
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name,
			Namespace:   cr.Namespace,
			Labels:      labels,
			Annotations: cr.Spec.Service.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return svc
}

func probePaths(cr *gatewayv1alpha1.OmniRoute) (liveness, readiness, startup string) {
	liveness = defaultLivenessPath
	readiness = defaultHealthPath
	startup = defaultHealthPath

	if cr.Spec.Probes != nil {
		if cr.Spec.Probes.LivenessPath != "" {
			liveness = cr.Spec.Probes.LivenessPath
		}
		if cr.Spec.Probes.ReadinessPath != "" {
			readiness = cr.Spec.Probes.ReadinessPath
		}
		if cr.Spec.Probes.StartupPath != "" {
			startup = cr.Spec.Probes.StartupPath
		}
	}

	return
}

func buildEnvVars(cr *gatewayv1alpha1.OmniRoute, port int32) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{Name: "PORT", Value: fmt.Sprintf("%d", port)},
		{Name: "HOSTNAME", Value: "0.0.0.0"},
		{Name: "DATA_DIR", Value: dataMountPath},
		{Name: "REQUIRE_API_KEY", Value: "true"},
	}

	// Auto-derive NODE_OPTIONS from memory limits to prevent OOM kills.
	if cr.Spec.Resources.Limits != nil {
		if memLimit, ok := cr.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
			memMB := memLimit.Value() / (1024 * 1024)
			// Use 75% of the memory limit for the V8 heap.
			heapMB := memMB * 75 / 100
			if heapMB > 0 {
				envs = append(envs, corev1.EnvVar{
					Name:  "NODE_OPTIONS",
					Value: "--max-old-space-size=" + strconv.FormatInt(heapMB, 10),
				})
			}
		}
	}

	// Set OMNIROUTE_BASE_PATH if configured.
	if cr.Spec.BasePath != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  "OMNIROUTE_BASE_PATH",
			Value: cr.Spec.BasePath,
		})
	}

	// Inject credentials from existing Secret.
	if cr.Spec.Credentials.ExistingSecret != "" {
		secretKeys := []string{"JWT_SECRET", "API_KEY_SECRET", "INITIAL_PASSWORD"}
		for _, key := range secretKeys {
			envs = append(envs, corev1.EnvVar{
				Name: key,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.Spec.Credentials.ExistingSecret,
						},
						Key: key,
					},
				},
			})
		}
	}

	// Redis configuration.
	if cr.Spec.Redis != nil {
		if cr.Spec.Redis.ExistingSecret != "" {
			envs = append(envs, corev1.EnvVar{
				Name: envRedisURL,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.Spec.Redis.ExistingSecret,
						},
						Key: envRedisURL,
					},
				},
			})
		} else if cr.Spec.Redis.URL != "" {
			envs = append(envs, corev1.EnvVar{
				Name:  envRedisURL,
				Value: cr.Spec.Redis.URL,
			})
		}
		if cr.Spec.Redis.KeyPrefix != "" {
			envs = append(envs, corev1.EnvVar{
				Name:  "REDIS_KEY_PREFIX",
				Value: cr.Spec.Redis.KeyPrefix,
			})
		}
	}

	// Database encryption.
	if cr.Spec.Encryption != nil {
		if cr.Spec.Encryption.ExistingSecret != "" {
			envs = append(envs, corev1.EnvVar{
				Name: "STORAGE_ENCRYPTION_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.Spec.Encryption.ExistingSecret,
						},
						Key: "STORAGE_ENCRYPTION_KEY",
					},
				},
			})
		}
		if cr.Spec.Encryption.KeyVersion != "" {
			envs = append(envs, corev1.EnvVar{
				Name:  "STORAGE_ENCRYPTION_KEY_VERSION",
				Value: cr.Spec.Encryption.KeyVersion,
			})
		}
	}

	envs = append(envs, cr.Spec.Env...)
	return envs
}

func buildEnvFrom(cr *gatewayv1alpha1.OmniRoute) []corev1.EnvFromSource {
	if cr.Spec.Config.ExistingConfigMap == "" {
		return nil
	}
	return []corev1.EnvFromSource{
		{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Spec.Config.ExistingConfigMap,
				},
				Optional: boolPtr(true),
			},
		},
	}
}

func buildVolumes(cr *gatewayv1alpha1.OmniRoute) ([]corev1.Volume, []corev1.VolumeMount) {
	persistEnabled := cr.Spec.Persistence.Enabled == nil || *cr.Spec.Persistence.Enabled

	if !persistEnabled {
		vol := corev1.Volume{
			Name: dataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		mount := corev1.VolumeMount{
			Name:      dataVolumeName,
			MountPath: dataMountPath,
		}
		return []corev1.Volume{vol}, []corev1.VolumeMount{mount}
	}

	vol := corev1.Volume{
		Name: dataVolumeName,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName(cr.Name),
			},
		},
	}

	mount := corev1.VolumeMount{
		Name:      dataVolumeName,
		MountPath: dataMountPath,
	}

	return []corev1.Volume{vol}, []corev1.VolumeMount{mount}
}

func labelsForOmniRoute(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "omniroute",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "omniroute-operator",
	}
}

func imageString(cr *gatewayv1alpha1.OmniRoute) string {
	repo := cr.Spec.Image.Repository
	if repo == "" {
		repo = defaultImage
	}
	tag := cr.Spec.Image.Tag
	if tag == "" {
		tag = defaultTag
	}
	return repo + ":" + tag
}

func servicePort(cr *gatewayv1alpha1.OmniRoute) int32 {
	if cr.Spec.Service.Port > 0 {
		return cr.Spec.Service.Port
	}
	return defaultPort
}

func pvcName(crName string) string {
	return crName + "-data"
}

func (r *OmniRouteReconciler) updateStatusFromDeployment(ctx context.Context, cr *gatewayv1alpha1.OmniRoute) {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, dep)
	if err != nil {
		return
	}

	cr.Status.ReadyReplicas = dep.Status.ReadyReplicas
	cr.Status.ObservedGeneration = cr.Generation

	if dep.Status.ReadyReplicas > 0 && dep.Status.ReadyReplicas == dep.Status.Replicas {
		cr.Status.Phase = "Running"
		r.setCondition(cr, gatewayv1alpha1.ConditionReady, metav1.ConditionTrue, "DeploymentReady", "All replicas are ready")
		r.setCondition(cr, gatewayv1alpha1.ConditionProgressing, metav1.ConditionFalse, "ReconcileComplete", "Reconciliation finished")
		r.setCondition(cr, gatewayv1alpha1.ConditionDegraded, metav1.ConditionFalse, "NotDegraded", "All resources healthy")
	} else {
		cr.Status.Phase = "Progressing"
		r.setCondition(cr, gatewayv1alpha1.ConditionReady, metav1.ConditionFalse, "DeploymentNotReady", "Waiting for replicas")
	}
}

func (r *OmniRouteReconciler) setCondition(cr *gatewayv1alpha1.OmniRoute, condType gatewayv1alpha1.ConditionType, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(condType),
		Status:             status,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	for i, c := range cr.Status.Conditions {
		if c.Type == string(condType) {
			if c.Status != status {
				cr.Status.Conditions[i] = condition
			}
			return
		}
	}
	cr.Status.Conditions = append(cr.Status.Conditions, condition)
}

func (r *OmniRouteReconciler) statusUpdate(ctx context.Context, cr *gatewayv1alpha1.OmniRoute) error {
	return r.Status().Update(ctx, cr)
}

func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

// SetupWithManager sets up the controller with the Manager.
func (r *OmniRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.OmniRoute{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.Ingress{}).
		Named("omniroute").
		Complete(r)
}
