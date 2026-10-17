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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha1 "github.com/vinaycharlie01/omniroute-operator/api/v1alpha1"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = gatewayv1alpha1.AddToScheme(s)
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
			Namespace: "default",
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

func TestReconcile_ExistingResource(t *testing.T) {
	scheme := newScheme()

	replicas := int32(1)
	cr := &gatewayv1alpha1.OmniRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-omniroute",
			Namespace: "default",
		},
		Spec: gatewayv1alpha1.OmniRouteSpec{
			Image: gatewayv1alpha1.ImageSpec{
				Repository: "diegosouzapw/omniroute",
				Tag:        "3.8.51",
			},
			Replicas: &replicas,
			Service: gatewayv1alpha1.ServiceSpec{
				Port: 20128,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		Build()

	r := &OmniRouteReconciler{
		Client: client,
		Scheme: scheme,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-omniroute",
			Namespace: "default",
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
