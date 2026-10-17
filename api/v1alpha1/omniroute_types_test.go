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
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	testGroup   = "gateway.omniroute.ai"
	testVersion = "v1alpha1"
	testKind    = "OmniRoute"
)

func TestOmniRouteGroupVersionKind(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   testGroup,
		Version: testVersion,
		Kind:    testKind,
	}
	if GroupVersion.Group != gvk.Group {
		t.Errorf("Group = %q, want %q", GroupVersion.Group, gvk.Group)
	}
	if GroupVersion.Version != gvk.Version {
		t.Errorf("Version = %q, want %q", GroupVersion.Version, gvk.Version)
	}
}

func TestSchemeRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	gvk := schema.GroupVersionKind{
		Group:   testGroup,
		Version: testVersion,
		Kind:    testKind,
	}
	obj, err := scheme.New(gvk)
	if err != nil {
		t.Fatalf("scheme.New(%v) error = %v", gvk, err)
	}
	if _, ok := obj.(*OmniRoute); !ok {
		t.Errorf("scheme.New(%v) returned %T, want *OmniRoute", gvk, obj)
	}

	listGVK := schema.GroupVersionKind{
		Group:   testGroup,
		Version: testVersion,
		Kind:    "OmniRouteList",
	}
	listObj, err := scheme.New(listGVK)
	if err != nil {
		t.Fatalf("scheme.New(%v) error = %v", listGVK, err)
	}
	if _, ok := listObj.(*OmniRouteList); !ok {
		t.Errorf("scheme.New(%v) returned %T, want *OmniRouteList", listGVK, listObj)
	}
}

func TestOmniRouteSpecDefaults(t *testing.T) {
	spec := OmniRouteSpec{}

	if spec.Image.Repository != "" {
		t.Errorf("default Image.Repository = %q, want empty (defaults applied by webhook/CRD)", spec.Image.Repository)
	}
}

func TestOmniRouteSpecFullRoundTrip(t *testing.T) {
	replicas := int32(1)
	persistEnabled := true
	storageClass := "standard"

	original := &OmniRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.omniroute.ai/v1alpha1",
			Kind:       "OmniRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-omniroute",
			Namespace: "ai-platform",
		},
		Spec: OmniRouteSpec{
			Image: ImageSpec{
				Repository: "diegosouzapw/omniroute",
				Tag:        "3.8.51",
				PullPolicy: corev1.PullIfNotPresent,
			},
			Replicas: &replicas,
			Service: ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Port: 20128,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Config: ConfigSpec{
				ExistingConfigMap: "omniroute-config",
			},
			Credentials: CredentialsSpec{
				ExistingSecret: "omniroute-secrets",
			},
			Persistence: PersistenceSpec{
				Enabled:      &persistEnabled,
				StorageClass: &storageClass,
				Size:         resource.MustParse("1Gi"),
				AccessMode:   corev1.ReadWriteOnce,
			},
			Env: []corev1.EnvVar{
				{Name: "APP_LOG_LEVEL", Value: "info"},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	roundtripped := &OmniRoute{}
	if err := json.Unmarshal(data, roundtripped); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if roundtripped.Name != original.Name {
		t.Errorf("Name = %q, want %q", roundtripped.Name, original.Name)
	}
	if roundtripped.Namespace != original.Namespace {
		t.Errorf("Namespace = %q, want %q", roundtripped.Namespace, original.Namespace)
	}
	if roundtripped.Spec.Image.Repository != original.Spec.Image.Repository {
		t.Errorf("Image.Repository = %q, want %q", roundtripped.Spec.Image.Repository, original.Spec.Image.Repository)
	}
	if roundtripped.Spec.Image.Tag != original.Spec.Image.Tag {
		t.Errorf("Image.Tag = %q, want %q", roundtripped.Spec.Image.Tag, original.Spec.Image.Tag)
	}
	if *roundtripped.Spec.Replicas != *original.Spec.Replicas {
		t.Errorf("Replicas = %d, want %d", *roundtripped.Spec.Replicas, *original.Spec.Replicas)
	}
	if roundtripped.Spec.Service.Port != original.Spec.Service.Port {
		t.Errorf("Service.Port = %d, want %d", roundtripped.Spec.Service.Port, original.Spec.Service.Port)
	}
	if roundtripped.Spec.Credentials.ExistingSecret != original.Spec.Credentials.ExistingSecret {
		t.Errorf("Credentials.ExistingSecret = %q, want %q",
			roundtripped.Spec.Credentials.ExistingSecret, original.Spec.Credentials.ExistingSecret)
	}
	if !roundtripped.Spec.Persistence.Size.Equal(original.Spec.Persistence.Size) {
		t.Errorf("Persistence.Size = %v, want %v",
			roundtripped.Spec.Persistence.Size, original.Spec.Persistence.Size)
	}
	if len(roundtripped.Spec.Env) != 1 || roundtripped.Spec.Env[0].Name != "APP_LOG_LEVEL" {
		t.Errorf("Env = %v, want [{APP_LOG_LEVEL info}]", roundtripped.Spec.Env)
	}
}

func TestOmniRouteDeepCopy(t *testing.T) {
	replicas := int32(1)
	persistEnabled := true

	original := &OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: OmniRouteSpec{
			Image: ImageSpec{
				Repository: "diegosouzapw/omniroute",
				Tag:        "3.8.51",
			},
			Replicas: &replicas,
			Persistence: PersistenceSpec{
				Enabled: &persistEnabled,
				Size:    resource.MustParse("5Gi"),
			},
		},
	}

	copied := original.DeepCopy()

	if copied.Name != original.Name {
		t.Errorf("DeepCopy().Name = %q, want %q", copied.Name, original.Name)
	}
	if copied.Spec.Image.Tag != original.Spec.Image.Tag {
		t.Errorf("DeepCopy().Spec.Image.Tag = %q, want %q", copied.Spec.Image.Tag, original.Spec.Image.Tag)
	}

	*copied.Spec.Replicas = 99
	if *original.Spec.Replicas == 99 {
		t.Error("DeepCopy shares pointer with original — Replicas was mutated")
	}

	*copied.Spec.Persistence.Enabled = false
	if !*original.Spec.Persistence.Enabled {
		t.Error("DeepCopy shares pointer with original — Persistence.Enabled was mutated")
	}
}

func TestOmniRouteStatusConditions(t *testing.T) {
	status := OmniRouteStatus{
		ReadyReplicas:      1,
		ObservedGeneration: 5,
		Phase:              "Running",
		Conditions: []metav1.Condition{
			{
				Type:   string(ConditionReady),
				Status: metav1.ConditionTrue,
				Reason: "AllReplicasReady",
			},
			{
				Type:   string(ConditionProgressing),
				Status: metav1.ConditionFalse,
				Reason: "DeploymentComplete",
			},
		},
	}

	if status.ReadyReplicas != 1 {
		t.Errorf("ReadyReplicas = %d, want 1", status.ReadyReplicas)
	}
	if len(status.Conditions) != 2 {
		t.Fatalf("len(Conditions) = %d, want 2", len(status.Conditions))
	}
	if status.Conditions[0].Type != string(ConditionReady) {
		t.Errorf("Conditions[0].Type = %q, want %q", status.Conditions[0].Type, ConditionReady)
	}
}

func TestConditionTypeConstants(t *testing.T) {
	tests := []struct {
		ct   ConditionType
		want string
	}{
		{ConditionReady, "Ready"},
		{ConditionProgressing, "Progressing"},
		{ConditionDegraded, "Degraded"},
	}
	for _, tt := range tests {
		if string(tt.ct) != tt.want {
			t.Errorf("ConditionType = %q, want %q", tt.ct, tt.want)
		}
	}
}

func TestMinimalSpec(t *testing.T) {
	cr := &OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal",
			Namespace: "default",
		},
		Spec: OmniRouteSpec{},
	}

	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	parsed := &OmniRoute{}
	if err := json.Unmarshal(data, parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if parsed.Spec.Replicas != nil {
		t.Errorf("minimal spec Replicas = %v, want nil", parsed.Spec.Replicas)
	}
	if parsed.Spec.Image.Repository != "" {
		t.Errorf("minimal spec Image.Repository = %q, want empty", parsed.Spec.Image.Repository)
	}
}
