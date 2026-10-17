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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	defaultImage   = "diegosouzapw/omniroute"
	defaultTag     = "3.8.51"
	defaultPort    = int32(20128)
	containerName  = "omniroute"
	dataVolumeName = "data"
	dataMountPath  = "/app/data"
	finalizerName  = "gateway.omniroute.ai/finalizer"
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

	if err := r.reconcileDeployment(ctx, omniroute); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, omniroute); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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

func (r *OmniRouteReconciler) buildDeployment(cr *gatewayv1alpha1.OmniRoute) *appsv1.Deployment {
	labels := labelsForOmniRoute(cr.Name)
	replicas := int32(1)
	if cr.Spec.Replicas != nil {
		replicas = *cr.Spec.Replicas
	}

	image := imageString(cr)
	port := servicePort(cr)

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
		Env: buildEnvVars(cr, port),
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/livez",
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       15,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
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
					Path: "/healthz",
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

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    labels,
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
}

func buildEnvVars(cr *gatewayv1alpha1.OmniRoute, port int32) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{Name: "PORT", Value: fmt.Sprintf("%d", port)},
		{Name: "HOSTNAME", Value: "0.0.0.0"},
		{Name: "DATA_DIR", Value: dataMountPath},
		{Name: "REQUIRE_API_KEY", Value: "true"},
	}

	if cr.Spec.Config.ExistingConfigMap != "" {
		envs = append(envs, corev1.EnvVar{
			Name: "OMNIROUTE_CONFIG_LOADED",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.Config.ExistingConfigMap,
					},
					Key:      "PORT",
					Optional: boolPtr(true),
				},
			},
		})
	}

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

	envs = append(envs, cr.Spec.Env...)
	return envs
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

	size := cr.Spec.Persistence.Size
	if size.IsZero() {
		size = resource.MustParse("1Gi")
	}

	accessMode := corev1.ReadWriteOnce
	if cr.Spec.Persistence.AccessMode != "" {
		accessMode = cr.Spec.Persistence.AccessMode
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

	_ = size
	_ = accessMode

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

func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

// SetupWithManager sets up the controller with the Manager.
func (r *OmniRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.OmniRoute{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("omniroute").
		Complete(r)
}
