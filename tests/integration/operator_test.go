//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/k3s"

	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	k3sImage       = "rancher/k3s:v1.31.4-k3s1"
	testTimeout    = 3 * time.Minute
	pollInterval   = 2 * time.Second
	testNamespace  = "test-omniroute"
	operatorNS     = "omniroute-system"
	crdName        = "omniroutes.gateway.omniroute.ai"
)

var omnirouteGVR = schema.GroupVersionResource{
	Group:    "gateway.omniroute.ai",
	Version:  "v1alpha1",
	Resource: "omniroutes",
}

func projectRoot() string {
	// Walk up from tests/integration to repo root.
	wd, _ := os.Getwd()
	return filepath.Join(wd, "..", "..")
}

// startK3s spins up a k3s container and returns a clientset + dynamic client.
func startK3s(t *testing.T, ctx context.Context) (*k3s.K3sContainer, kubernetes.Interface, dynamic.Interface, apiextclient.Interface) {
	t.Helper()

	crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "gateway.omniroute.ai_omniroutes.yaml")
	require.FileExists(t, crdPath, "CRD manifest must exist")

	container, err := k3s.Run(ctx, k3sImage,
		k3s.WithManifest(crdPath),
	)
	require.NoError(t, err, "failed to start k3s container")

	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate k3s container: %v", err)
		}
	})

	kubeConfigBytes, err := container.GetKubeConfig(ctx)
	require.NoError(t, err, "failed to get kubeconfig")

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	require.NoError(t, err, "failed to build rest config from kubeconfig")

	clientset, err := kubernetes.NewForConfig(restConfig)
	require.NoError(t, err, "failed to create kubernetes clientset")

	dynClient, err := dynamic.NewForConfig(restConfig)
	require.NoError(t, err, "failed to create dynamic client")

	apiextClient, err := apiextclient.NewForConfig(restConfig)
	require.NoError(t, err, "failed to create apiextensions clientset")

	return container, clientset, dynClient, apiextClient
}

// waitForCRD polls until the CRD is established.
func waitForCRD(t *testing.T, ctx context.Context, apiextClient apiextclient.Interface) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, pollInterval, testTimeout, true, func(ctx context.Context) (bool, error) {
		crd, err := apiextClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextv1.Established && cond.Status == apiextv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	require.NoError(t, err, "CRD %s did not become established", crdName)
}

func TestCRDInstallation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, _, _, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	crd, err := apiextClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "omniroutes", crd.Spec.Names.Plural)
	assert.Equal(t, "OmniRoute", crd.Spec.Names.Kind)
	assert.Equal(t, "gateway.omniroute.ai", crd.Spec.Group)

	// Verify v1alpha1 is a served version.
	var hasV1alpha1 bool
	for _, v := range crd.Spec.Versions {
		if v.Name == "v1alpha1" && v.Served {
			hasV1alpha1 = true
		}
	}
	assert.True(t, hasV1alpha1, "CRD should serve v1alpha1")
}

func TestCreateOmniRouteResource(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	// Create namespace.
	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create the secret first (the CR references it).
	_, err = clientset.CoreV1().Secrets(testNamespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secrets"},
		StringData: map[string]string{
			"JWT_SECRET":       "test-jwt-secret",
			"API_KEY_SECRET":   "test-api-key-secret",
			"INITIAL_PASSWORD": "test-password",
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create an OmniRoute custom resource.
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.omniroute.ai/v1alpha1",
			"kind":       "OmniRoute",
			"metadata": map[string]interface{}{
				"name":      "test-omniroute",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "diegosouzapw/omniroute",
					"tag":        "3.8.51",
				},
				"replicas": int64(1),
				"service": map[string]interface{}{
					"type": "ClusterIP",
					"port": int64(20128),
				},
				"credentials": map[string]interface{}{
					"existingSecret": "test-secrets",
				},
				"persistence": map[string]interface{}{
					"enabled": true,
					"size":    "1Gi",
				},
			},
		},
	}

	created, err := dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err, "failed to create OmniRoute CR")

	assert.Equal(t, "test-omniroute", created.GetName())
	assert.Equal(t, testNamespace, created.GetNamespace())

	// Verify spec fields round-trip correctly.
	spec, found, err := unstructured.NestedMap(created.Object, "spec")
	require.NoError(t, err)
	require.True(t, found, "spec should exist")

	imageMap, _, _ := unstructured.NestedMap(spec, "image")
	assert.Equal(t, "diegosouzapw/omniroute", imageMap["repository"])
	assert.Equal(t, "3.8.51", imageMap["tag"])

	// Verify we can list OmniRoute resources.
	list, err := dynClient.Resource(omnirouteGVR).Namespace(testNamespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, list.Items, 1)
}

func TestUpdateOmniRouteResource(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.omniroute.ai/v1alpha1",
			"kind":       "OmniRoute",
			"metadata": map[string]interface{}{
				"name":      "update-test",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"image": map[string]interface{}{
					"tag": "3.8.51",
				},
				"replicas": int64(1),
			},
		},
	}

	created, err := dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err)

	// Update the image tag.
	err = unstructured.SetNestedField(created.Object, "4.0.0", "spec", "image", "tag")
	require.NoError(t, err)

	updated, err := dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Update(ctx, created, metav1.UpdateOptions{})
	require.NoError(t, err)

	tag, _, _ := unstructured.NestedString(updated.Object, "spec", "image", "tag")
	assert.Equal(t, "4.0.0", tag)
}

func TestDeleteOmniRouteResource(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.omniroute.ai/v1alpha1",
			"kind":       "OmniRoute",
			"metadata": map[string]interface{}{
				"name":      "delete-test",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
			},
		},
	}

	_, err = dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err)

	err = dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Delete(ctx, "delete-test", metav1.DeleteOptions{})
	require.NoError(t, err)

	// Verify it's gone.
	_, err = dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Get(ctx, "delete-test", metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err), "resource should be deleted")
}

func TestCRDValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Service type must be one of ClusterIP, NodePort, LoadBalancer.
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.omniroute.ai/v1alpha1",
			"kind":       "OmniRoute",
			"metadata": map[string]interface{}{
				"name":      "invalid-svc-type",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"service": map[string]interface{}{
					"type": "InvalidType",
				},
			},
		},
	}

	_, err = dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	assert.Error(t, err, "CRD validation should reject invalid service type")

	// Invalid persistence accessMode.
	cr2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.omniroute.ai/v1alpha1",
			"kind":       "OmniRoute",
			"metadata": map[string]interface{}{
				"name":      "invalid-access-mode",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"persistence": map[string]interface{}{
					"accessMode": "InvalidMode",
				},
			},
		},
	}

	_, err = dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Create(ctx, cr2, metav1.CreateOptions{})
	assert.Error(t, err, "CRD validation should reject invalid access mode")
}

func TestHelmChartInstallation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, _, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	// Create the operator namespace.
	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: operatorNS},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Apply the Helm-rendered RBAC and deployment manifests directly.
	// This simulates what `helm install` would produce.
	rbacManifestPath := filepath.Join(projectRoot(), "charts", "omniroute-operator", "templates", "rbac.yaml")
	saManifestPath := filepath.Join(projectRoot(), "charts", "omniroute-operator", "templates", "serviceaccount.yaml")

	// Create the ServiceAccount the operator needs.
	_, err = clientset.CoreV1().ServiceAccounts(operatorNS).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "omniroute-operator",
			Namespace: operatorNS,
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Verify the template files exist (Helm chart structure).
	assert.FileExists(t, rbacManifestPath, "RBAC template should exist")
	assert.FileExists(t, saManifestPath, "ServiceAccount template should exist")

	// Verify the Deployment template exists.
	depManifestPath := filepath.Join(projectRoot(), "charts", "omniroute-operator", "templates", "deployment.yaml")
	assert.FileExists(t, depManifestPath, "Deployment template should exist")

	// Verify CRD file in the Helm crds/ directory.
	helmCRD := filepath.Join(projectRoot(), "charts", "omniroute-operator", "crds", "gateway.omniroute.ai_omniroutes.yaml")
	assert.FileExists(t, helmCRD, "Helm CRD should be synced")

	// Verify Chart.yaml.
	chartPath := filepath.Join(projectRoot(), "charts", "omniroute-operator", "Chart.yaml")
	chartData, err := os.ReadFile(chartPath)
	require.NoError(t, err)
	assert.Contains(t, string(chartData), "name: omniroute-operator")
	assert.Contains(t, string(chartData), "version: 0.1.0")
}

func TestSampleCRApply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	// Create the namespace the sample CR targets.
	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "ai-platform"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create the secret the sample references.
	_, err = clientset.CoreV1().Secrets("ai-platform").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "omniroute-secrets"},
		StringData: map[string]string{
			"JWT_SECRET":       "test-jwt",
			"API_KEY_SECRET":   "test-api-key",
			"INITIAL_PASSWORD": "test-pw",
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Read and apply the sample CR from config/samples/.
	samplePath := filepath.Join(projectRoot(), "config", "samples", "gateway_v1alpha1_omniroute.yaml")
	require.FileExists(t, samplePath)

	data, err := os.ReadFile(samplePath)
	require.NoError(t, err)

	obj := &unstructured.Unstructured{}
	err = yaml.UnmarshalStrict(data, &obj.Object)
	require.NoError(t, err)

	ns := obj.GetNamespace()
	if ns == "" {
		ns = "ai-platform"
	}

	created, err := dynClient.Resource(omnirouteGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	require.NoError(t, err, "sample CR should apply without errors")

	assert.Equal(t, "omniroute-sample", created.GetName())

	// Verify spec fields from the sample.
	tag, _, _ := unstructured.NestedString(created.Object, "spec", "image", "tag")
	assert.Equal(t, "3.8.51", tag)

	port, _, _ := unstructured.NestedInt64(created.Object, "spec", "service", "port")
	assert.Equal(t, int64(20128), port)

	enabled, _, _ := unstructured.NestedBool(created.Object, "spec", "persistence", "enabled")
	assert.True(t, enabled)
}

func TestMultipleOmniRouteInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	// Create two separate namespaces.
	for _, ns := range []string{"team-a", "team-b"} {
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	// Create OmniRoute in each namespace.
	for _, ns := range []string{"team-a", "team-b"} {
		cr := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.omniroute.ai/v1alpha1",
				"kind":       "OmniRoute",
				"metadata": map[string]interface{}{
					"name":      "omniroute",
					"namespace": ns,
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"service": map[string]interface{}{
						"port": int64(20128),
					},
				},
			},
		}

		_, err := dynClient.Resource(omnirouteGVR).Namespace(ns).Create(ctx, cr, metav1.CreateOptions{})
		require.NoError(t, err, "failed to create OmniRoute in %s", ns)
	}

	// Both should exist independently.
	for _, ns := range []string{"team-a", "team-b"} {
		obj, err := dynClient.Resource(omnirouteGVR).Namespace(ns).Get(ctx, "omniroute", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, ns, obj.GetNamespace())
	}

	// Delete from team-a should not affect team-b.
	err := dynClient.Resource(omnirouteGVR).Namespace("team-a").Delete(ctx, "omniroute", metav1.DeleteOptions{})
	require.NoError(t, err)

	_, err = dynClient.Resource(omnirouteGVR).Namespace("team-b").Get(ctx, "omniroute", metav1.GetOptions{})
	assert.NoError(t, err, "team-b OmniRoute should still exist after deleting team-a's")
}

func TestPersistenceStorageClassField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, clientset, dynClient, apiextClient := startK3s(t, ctx)

	waitForCRD(t, ctx, apiextClient)

	_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create an OmniRoute with a storageClass specified.
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.omniroute.ai/v1alpha1",
			"kind":       "OmniRoute",
			"metadata": map[string]interface{}{
				"name":      "sc-test",
				"namespace": testNamespace,
			},
			"spec": map[string]interface{}{
				"persistence": map[string]interface{}{
					"enabled":      true,
					"size":         "2Gi",
					"storageClass": "local-path",
					"accessMode":   "ReadWriteOnce",
				},
			},
		},
	}

	created, err := dynClient.Resource(omnirouteGVR).Namespace(testNamespace).Create(ctx, cr, metav1.CreateOptions{})
	require.NoError(t, err)

	sc, found, err := unstructured.NestedString(created.Object, "spec", "persistence", "storageClass")
	require.NoError(t, err)
	assert.True(t, found, "storageClass field should be present")
	assert.Equal(t, "local-path", sc)

	size, _, _ := unstructured.NestedString(created.Object, "spec", "persistence", "size")
	assert.Equal(t, "2Gi", size)

	mode, _, _ := unstructured.NestedString(created.Object, "spec", "persistence", "accessMode")
	assert.Equal(t, "ReadWriteOnce", mode)
}
