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
	"k8s.io/apimachinery/pkg/api/errors"
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
	testNamespace    = "default"
	testName         = "test-omniroute"
	testDefaultImage = "diegosouzapw/omniroute:3.8.51"
	testMinimalName  = "minimal"
	testSecretName   = "with-secrets"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(gatewayv1alpha1.AddToScheme(s))
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
	if container.LivenessProbe.HTTPGet.Path != "/livez" {
		t.Errorf("LivenessProbe path = %q, want /livez", container.LivenessProbe.HTTPGet.Path)
	}
	if container.ReadinessProbe.HTTPGet.Path != "/healthz" {
		t.Errorf("ReadinessProbe path = %q, want /healthz", container.ReadinessProbe.HTTPGet.Path)
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
