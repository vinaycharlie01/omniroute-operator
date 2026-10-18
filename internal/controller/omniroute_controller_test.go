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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha1 "github.com/vinaycharlie01/omniroute-operator/api/v1alpha1"
)

const (
	testNamespace      = "default"
	testName           = "test-omniroute"
	testDefaultImage   = "diegosouzapw/omniroute:3.8.51"
	testMinimalName    = "minimal"
	testSecretName     = "with-secrets"
	testProbe       = "probe-test"
	testCustomProbe = "custom-probe"
	testEnvFrom     = "envfrom-test"
	testNodeOpts    = "node-opts"
	testNoNodeOpts  = "no-node-opts"
	testRedis       = "redis-test"
	testRedisSecret = "redis-secret"
	testEncryption  = "enc-test"
	testBasePath    = "basepath-test"
	testIngress     = "ingress-test"
	testNoIngress   = "no-ingress"
	testServiceAnn  = "svc-ann"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(gatewayv1alpha1.AddToScheme(s))
	utilruntime.Must(networkingv1.AddToScheme(s))
	return s
}

func TestReconcile_NotFound(t *testing.T) {
	scheme := newScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OmniRouteReconciler{
		Client: client,
		Scheme: scheme,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: testNamespace,
		},
	}

	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("Reconcile() result = %v, want empty", result)
	}
}

func TestReconcile_CreatesDeploymentAndService(t *testing.T) {
	scheme := newScheme()

	replicas := int32(1)
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Image: gatewayv1alpha1.ImageSpec{
				Repository: "diegosouzapw/omniroute",
				Tag:        "3.8.51",
				PullPolicy: corev1.PullIfNotPresent,
			},
			Replicas: &replicas,
			Service: gatewayv1alpha1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Port: 20128,
			},
			Credentials: gatewayv1alpha1.CredentialsSpec{
				ExistingSecret: "omniroute-secrets",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      testName,
			Namespace: testNamespace,
		},
	}

	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("Reconcile() result = %v, want empty", result)
	}

	dep := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName, Namespace: testNamespace,
	}, dep)
	if err != nil {
		t.Fatalf("expected Deployment to be created, got error: %v", err)
	}

	if *dep.Spec.Replicas != 1 {
		t.Errorf("Deployment replicas = %d, want 1", *dep.Spec.Replicas)
	}

	container := dep.Spec.Template.Spec.Containers[0]
	if container.Image != testDefaultImage {
		t.Errorf("Container image = %q, want %q", container.Image, testDefaultImage)
	}
	if container.Ports[0].ContainerPort != 20128 {
		t.Errorf("Container port = %d, want 20128", container.Ports[0].ContainerPort)
	}
	if container.LivenessProbe == nil {
		t.Error("expected LivenessProbe to be set")
	}
	if container.ReadinessProbe == nil {
		t.Error("expected ReadinessProbe to be set")
	}
	if container.StartupProbe == nil {
		t.Error("expected StartupProbe to be set")
	}
	if container.LivenessProbe.HTTPGet.Path != defaultHealthPath {
		t.Errorf("LivenessProbe path = %q, want %s", container.LivenessProbe.HTTPGet.Path, defaultHealthPath)
	}
	if container.ReadinessProbe.HTTPGet.Path != defaultHealthPath {
		t.Errorf("ReadinessProbe path = %q, want %s", container.ReadinessProbe.HTTPGet.Path, defaultHealthPath)
	}

	podSpec := dep.Spec.Template.Spec
	if podSpec.SecurityContext == nil || !*podSpec.SecurityContext.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true")
	}
	if *podSpec.TerminationGracePeriodSeconds != 40 {
		t.Errorf("TerminationGracePeriodSeconds = %d, want 40", *podSpec.TerminationGracePeriodSeconds)
	}

	svc := &corev1.Service{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName, Namespace: testNamespace,
	}, svc)
	if err != nil {
		t.Fatalf("expected Service to be created, got error: %v", err)
	}

	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("Service type = %q, want ClusterIP", svc.Spec.Type)
	}
	if svc.Spec.Ports[0].Port != 20128 {
		t.Errorf("Service port = %d, want 20128", svc.Spec.Ports[0].Port)
	}
}

func TestReconcile_UpdatesExistingDeployment(t *testing.T) {
	scheme := newScheme()

	replicas := int32(1)
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Image: gatewayv1alpha1.ImageSpec{
				Repository: "diegosouzapw/omniroute",
				Tag:        "4.0.0",
			},
			Replicas: &replicas,
		},
	}

	existingDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
			Labels:    labelsForOmniRoute(testName),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labelsForOmniRoute(testName)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labelsForOmniRoute(testName)},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "omniroute", Image: testDefaultImage},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr, existingDep).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName, Namespace: testNamespace,
	}, dep)

	if dep.Spec.Template.Spec.Containers[0].Image != "diegosouzapw/omniroute:4.0.0" {
		t.Errorf("image = %q, want diegosouzapw/omniroute:4.0.0",
			dep.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestReconcile_DefaultValues(t *testing.T) {
	scheme := newScheme()

	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testMinimalName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testMinimalName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testMinimalName, Namespace: testNamespace,
	}, dep)

	container := dep.Spec.Template.Spec.Containers[0]
	if container.Image != testDefaultImage {
		t.Errorf("default image = %q, want %s", container.Image, testDefaultImage)
	}

	svc := &corev1.Service{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testMinimalName, Namespace: testNamespace,
	}, svc)
	if svc.Spec.Ports[0].Port != 20128 {
		t.Errorf("default service port = %d, want 20128", svc.Spec.Ports[0].Port)
	}
}

func TestReconcile_EnvVarsFromSecret(t *testing.T) {
	scheme := newScheme()

	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Credentials: gatewayv1alpha1.CredentialsSpec{
				ExistingSecret: "my-secret",
			},
			Env: []corev1.EnvVar{
				{Name: "CUSTOM_VAR", Value: "custom-value"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testSecretName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testSecretName, Namespace: testNamespace,
	}, dep)

	envs := dep.Spec.Template.Spec.Containers[0].Env
	secretEnvNames := map[string]bool{}
	var customFound bool
	for _, e := range envs {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			secretEnvNames[e.Name] = true
			if e.ValueFrom.SecretKeyRef.Name != "my-secret" {
				t.Errorf("secret ref name = %q, want my-secret", e.ValueFrom.SecretKeyRef.Name)
			}
		}
		if e.Name == "CUSTOM_VAR" && e.Value == "custom-value" {
			customFound = true
		}
	}

	for _, key := range []string{"JWT_SECRET", "API_KEY_SECRET", "INITIAL_PASSWORD"} {
		if !secretEnvNames[key] {
			t.Errorf("expected env var %q from secret, not found", key)
		}
	}
	if !customFound {
		t.Error("expected custom env var CUSTOM_VAR=custom-value, not found")
	}
}

func TestReconcile_CreatesPVC(t *testing.T) {
	scheme := newScheme()

	replicas := int32(1)
	storageClass := "my-storage-class"
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Replicas: &replicas,
			Persistence: gatewayv1alpha1.PersistenceSpec{
				StorageClass: &storageClass,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName + "-data", Namespace: testNamespace,
	}, pvc)
	if err != nil {
		t.Fatalf("expected PVC to be created, got error: %v", err)
	}

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "my-storage-class" {
		t.Errorf("PVC storageClassName = %v, want my-storage-class", pvc.Spec.StorageClassName)
	}
	if pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("PVC accessMode = %v, want ReadWriteOnce", pvc.Spec.AccessModes[0])
	}
	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.String() != "1Gi" {
		t.Errorf("PVC size = %s, want 1Gi", storageReq.String())
	}
}

func TestReconcile_PVCDefaultStorageClass(t *testing.T) {
	scheme := newScheme()

	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName + "-data", Namespace: testNamespace,
	}, pvc)
	if err != nil {
		t.Fatalf("expected PVC to be created, got error: %v", err)
	}

	if pvc.Spec.StorageClassName != nil {
		t.Errorf("PVC storageClassName = %v, want nil (cluster default)", pvc.Spec.StorageClassName)
	}
}

func TestReconcile_NoPVCWhenPersistenceDisabled(t *testing.T) {
	scheme := newScheme()

	disabled := false
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Persistence: gatewayv1alpha1.PersistenceSpec{
				Enabled: &disabled,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName + "-data", Namespace: testNamespace,
	}, pvc)
	if !errors.IsNotFound(err) {
		t.Error("expected PVC to NOT be created when persistence is disabled")
	}
}

func TestReconcile_Finalizer(t *testing.T) {
	scheme := newScheme()

	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &gatewayv1alpha1.OmniRoute{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName, Namespace: testNamespace,
	}, updated)

	if !controllerutil.ContainsFinalizer(updated, finalizerName) {
		t.Error("expected finalizer to be added")
	}
}

func TestReconcile_StatusConditions(t *testing.T) {
	scheme := newScheme()

	replicas := int32(1)
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Replicas: &replicas,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()

	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &gatewayv1alpha1.OmniRoute{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: testName, Namespace: testNamespace,
	}, updated)

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected status conditions to be set")
	}

	var hasProgressing bool
	for _, c := range updated.Status.Conditions {
		if c.Type == string(gatewayv1alpha1.ConditionProgressing) {
			hasProgressing = true
		}
	}
	if !hasProgressing {
		t.Error("expected Progressing condition to be set")
	}
}

func TestReconcile_SetupWithManager(t *testing.T) {
	r := &OmniRouteReconciler{}
	if r.Client != nil {
		t.Error("expected nil Client on zero-value reconciler")
	}
}

func TestNotFoundError(t *testing.T) {
	err := errors.NewNotFound(schema.GroupResource{
		Group:    "gateway.omniroute.ai",
		Resource: "omniroutes",
	}, "test")

	if !errors.IsNotFound(err) {
		t.Error("expected IsNotFound to be true")
	}
}

func TestLabelsForOmniRoute(t *testing.T) {
	labels := labelsForOmniRoute("my-instance")
	if labels["app.kubernetes.io/name"] != "omniroute" {
		t.Errorf("label name = %q, want omniroute", labels["app.kubernetes.io/name"])
	}
	if labels["app.kubernetes.io/instance"] != "my-instance" {
		t.Errorf("label instance = %q, want my-instance", labels["app.kubernetes.io/instance"])
	}
	if labels["app.kubernetes.io/managed-by"] != "omniroute-operator" {
		t.Errorf("label managed-by = %q, want omniroute-operator", labels["app.kubernetes.io/managed-by"])
	}
}

func TestImageString(t *testing.T) {
	tests := []struct {
		name string
		spec gatewayv1alpha1.OmniRouteSpec
		want string
	}{
		{"defaults", gatewayv1alpha1.OmniRouteSpec{}, testDefaultImage},
		{"custom", gatewayv1alpha1.OmniRouteSpec{
			Image: gatewayv1alpha1.ImageSpec{Repository: "myrepo/omniroute", Tag: "latest"},
		}, "myrepo/omniroute:latest"},
		{"only-tag", gatewayv1alpha1.OmniRouteSpec{
			Image: gatewayv1alpha1.ImageSpec{Tag: "4.0.0"},
		}, "diegosouzapw/omniroute:4.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &gatewayv1alpha1.OmniRoute{Spec: tt.spec}
			got := imageString(cr)
			if got != tt.want {
				t.Errorf("imageString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServicePort(t *testing.T) {
	cr := &gatewayv1alpha1.OmniRoute{}
	if servicePort(cr) != 20128 {
		t.Errorf("default servicePort() = %d, want 20128", servicePort(cr))
	}
	cr.Spec.Service.Port = 8080
	if servicePort(cr) != 8080 {
		t.Errorf("custom servicePort() = %d, want 8080", servicePort(cr))
	}
}

func TestReconcile_DefaultHealthProbes(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testProbe, Namespace: testNamespace},
		Spec:       gatewayv1alpha1.OmniRouteSpec{},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testProbe, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testProbe, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	if container.LivenessProbe.HTTPGet.Path != defaultHealthPath {
		t.Errorf("default liveness path = %q, want %s", container.LivenessProbe.HTTPGet.Path, defaultHealthPath)
	}
	if container.ReadinessProbe.HTTPGet.Path != defaultHealthPath {
		t.Errorf("default readiness path = %q, want %s", container.ReadinessProbe.HTTPGet.Path, defaultHealthPath)
	}
	if container.StartupProbe.HTTPGet.Path != defaultHealthPath {
		t.Errorf("default startup path = %q, want %s", container.StartupProbe.HTTPGet.Path, defaultHealthPath)
	}
}

func TestReconcile_CustomProbes(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testCustomProbe, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Probes: &gatewayv1alpha1.ProbesSpec{
				LivenessPath:  "/custom/livez",
				ReadinessPath: "/custom/readyz",
				StartupPath:   "/custom/startupz",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testCustomProbe, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testCustomProbe, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	if container.LivenessProbe.HTTPGet.Path != "/custom/livez" {
		t.Errorf("liveness path = %q, want /custom/livez", container.LivenessProbe.HTTPGet.Path)
	}
	if container.ReadinessProbe.HTTPGet.Path != "/custom/readyz" {
		t.Errorf("readiness path = %q, want /custom/readyz", container.ReadinessProbe.HTTPGet.Path)
	}
	if container.StartupProbe.HTTPGet.Path != "/custom/startupz" {
		t.Errorf("startup path = %q, want /custom/startupz", container.StartupProbe.HTTPGet.Path)
	}
}

func TestReconcile_ConfigMapEnvFrom(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testEnvFrom, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Config: gatewayv1alpha1.ConfigSpec{
				ExistingConfigMap: "my-configmap",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testEnvFrom, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testEnvFrom, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	if len(container.EnvFrom) == 0 {
		t.Fatal("expected envFrom to be set")
	}
	if container.EnvFrom[0].ConfigMapRef == nil {
		t.Fatal("expected configMapRef in envFrom")
	}
	if container.EnvFrom[0].ConfigMapRef.Name != "my-configmap" {
		t.Errorf("configMapRef name = %q, want my-configmap", container.EnvFrom[0].ConfigMapRef.Name)
	}

	// Verify no single-key OMNIROUTE_CONFIG_LOADED env var exists.
	for _, e := range container.Env {
		if e.Name == "OMNIROUTE_CONFIG_LOADED" {
			t.Error("found OMNIROUTE_CONFIG_LOADED env — should use envFrom instead")
		}
	}
}

func TestReconcile_NodeOptionsFromMemoryLimit(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testNodeOpts, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testNodeOpts, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testNodeOpts, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	var nodeOpts string
	for _, e := range container.Env {
		if e.Name == "NODE_OPTIONS" {
			nodeOpts = e.Value
		}
	}
	// 1Gi = 1024 MB, 75% = 768 MB
	if nodeOpts != "--max-old-space-size=768" {
		t.Errorf("NODE_OPTIONS = %q, want --max-old-space-size=768", nodeOpts)
	}
}

func TestReconcile_NoNodeOptionsWithoutMemoryLimit(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testNoNodeOpts, Namespace: testNamespace},
		Spec:       gatewayv1alpha1.OmniRouteSpec{},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testNoNodeOpts, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testNoNodeOpts, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	for _, e := range container.Env {
		if e.Name == "NODE_OPTIONS" {
			t.Error("NODE_OPTIONS should not be set without memory limit")
		}
	}
}

func TestReconcile_RedisURL(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testRedis, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Redis: &gatewayv1alpha1.RedisSpec{
				URL:       "redis://redis:6379",
				KeyPrefix: "omniroute:",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testRedis, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testRedis, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	envMap := map[string]string{}
	for _, e := range container.Env {
		if e.Value != "" {
			envMap[e.Name] = e.Value
		}
	}

	if envMap["REDIS_URL"] != "redis://redis:6379" {
		t.Errorf("REDIS_URL = %q, want redis://redis:6379", envMap["REDIS_URL"])
	}
	if envMap["REDIS_KEY_PREFIX"] != "omniroute:" {
		t.Errorf("REDIS_KEY_PREFIX = %q, want omniroute:", envMap["REDIS_KEY_PREFIX"])
	}
}

func TestReconcile_RedisFromSecret(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testRedisSecret, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Redis: &gatewayv1alpha1.RedisSpec{
				ExistingSecret: "redis-creds",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testRedisSecret, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testRedisSecret, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	var found bool
	for _, e := range container.Env {
		if e.Name == "REDIS_URL" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			if e.ValueFrom.SecretKeyRef.Name != "redis-creds" {
				t.Errorf("REDIS_URL secret ref = %q, want redis-creds", e.ValueFrom.SecretKeyRef.Name)
			}
			found = true
		}
	}
	if !found {
		t.Error("REDIS_URL from secret not found")
	}
}

func TestReconcile_Encryption(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testEncryption, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Encryption: &gatewayv1alpha1.EncryptionSpec{
				ExistingSecret: "encryption-secret",
				KeyVersion:     "2",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testEncryption, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testEncryption, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	var encKeyFound, encVersionFound bool
	for _, e := range container.Env {
		if e.Name == "STORAGE_ENCRYPTION_KEY" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			if e.ValueFrom.SecretKeyRef.Name == "encryption-secret" {
				encKeyFound = true
			}
		}
		if e.Name == "STORAGE_ENCRYPTION_KEY_VERSION" && e.Value == "2" {
			encVersionFound = true
		}
	}
	if !encKeyFound {
		t.Error("STORAGE_ENCRYPTION_KEY from secret not found")
	}
	if !encVersionFound {
		t.Error("STORAGE_ENCRYPTION_KEY_VERSION not found")
	}
}

func TestReconcile_BasePath(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testBasePath, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			BasePath: "/omniroute",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testBasePath, Namespace: testNamespace},
	})

	dep := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testBasePath, Namespace: testNamespace}, dep)
	container := dep.Spec.Template.Spec.Containers[0]

	var found bool
	for _, e := range container.Env {
		if e.Name == "OMNIROUTE_BASE_PATH" && e.Value == "/omniroute" {
			found = true
		}
	}
	if !found {
		t.Error("OMNIROUTE_BASE_PATH not set")
	}
}

func TestReconcile_IngressCreation(t *testing.T) {
	scheme := newScheme()
	className := "nginx"
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testIngress, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Ingress: &gatewayv1alpha1.IngressSpec{
				Enabled:   true,
				ClassName: &className,
				Host:      "omniroute.example.com",
				TLS: &gatewayv1alpha1.IngressTLSSpec{
					SecretName: "tls-secret",
				},
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testIngress, Namespace: testNamespace},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	ing := &networkingv1.Ingress{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: testIngress, Namespace: testNamespace}, ing)
	if err != nil {
		t.Fatalf("expected Ingress to be created, got error: %v", err)
	}

	if *ing.Spec.IngressClassName != "nginx" {
		t.Errorf("IngressClassName = %q, want nginx", *ing.Spec.IngressClassName)
	}
	if ing.Spec.Rules[0].Host != "omniroute.example.com" {
		t.Errorf("Ingress host = %q, want omniroute.example.com", ing.Spec.Rules[0].Host)
	}
	if len(ing.Spec.TLS) == 0 || ing.Spec.TLS[0].SecretName != "tls-secret" {
		t.Error("expected TLS with tls-secret")
	}
	if ing.Annotations["nginx.ingress.kubernetes.io/rewrite-target"] != "/" {
		t.Error("expected nginx rewrite annotation")
	}
}

func TestReconcile_NoIngressByDefault(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testNoIngress, Namespace: testNamespace},
		Spec:       gatewayv1alpha1.OmniRouteSpec{},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testNoIngress, Namespace: testNamespace},
	})

	ing := &networkingv1.Ingress{}
	err := fakeClient.Get(context.Background(), types.NamespacedName{Name: testNoIngress, Namespace: testNamespace}, ing)
	if !errors.IsNotFound(err) {
		t.Error("expected no Ingress to be created by default")
	}
}

func TestReconcile_ServiceAnnotations(t *testing.T) {
	scheme := newScheme()
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{Name: testServiceAnn, Namespace: testNamespace},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Service: gatewayv1alpha1.ServiceSpec{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).WithStatusSubresource(cr).Build()
	r := &OmniRouteReconciler{Client: fakeClient, Scheme: scheme}
	_, _ = r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: testServiceAnn, Namespace: testNamespace},
	})

	svc := &corev1.Service{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: testServiceAnn, Namespace: testNamespace}, svc)

	if svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"] != "nlb" {
		t.Error("expected Service annotation for AWS NLB")
	}
}

func TestProbePaths(t *testing.T) {
	tests := []struct {
		name      string
		probes    *gatewayv1alpha1.ProbesSpec
		wantLive  string
		wantReady string
		wantStart string
	}{
		{
			"nil probes",
			nil,
			defaultHealthPath,
			defaultHealthPath,
			defaultHealthPath,
		},
		{
			"partial override",
			&gatewayv1alpha1.ProbesSpec{LivenessPath: "/custom"},
			"/custom",
			defaultHealthPath,
			defaultHealthPath,
		},
		{
			"full override",
			&gatewayv1alpha1.ProbesSpec{
				LivenessPath:  "/live",
				ReadinessPath: "/ready",
				StartupPath:   "/start",
			},
			"/live",
			"/ready",
			"/start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &gatewayv1alpha1.OmniRoute{
				Spec: gatewayv1alpha1.OmniRouteSpec{Probes: tt.probes},
			}
			live, ready, start := probePaths(cr)
			if live != tt.wantLive {
				t.Errorf("liveness = %q, want %q", live, tt.wantLive)
			}
			if ready != tt.wantReady {
				t.Errorf("readiness = %q, want %q", ready, tt.wantReady)
			}
			if start != tt.wantStart {
				t.Errorf("startup = %q, want %q", start, tt.wantStart)
			}
		})
	}
}

func TestBuildEnvFrom(t *testing.T) {
	cr := &gatewayv1alpha1.OmniRoute{Spec: gatewayv1alpha1.OmniRouteSpec{}}
	if envFrom := buildEnvFrom(cr); envFrom != nil {
		t.Error("expected nil envFrom when no configmap set")
	}

	cr.Spec.Config.ExistingConfigMap = "my-cm"
	envFrom := buildEnvFrom(cr)
	if len(envFrom) != 1 {
		t.Fatalf("expected 1 envFrom, got %d", len(envFrom))
	}
	if envFrom[0].ConfigMapRef.Name != "my-cm" {
		t.Errorf("configMapRef name = %q, want my-cm", envFrom[0].ConfigMapRef.Name)
	}
}
