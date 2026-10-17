//go:build mage

// Mage build file for omniroute-operator.
// Powered by nava (https://github.com/nirantaraai/nava).
//
// Usage:
//
//	go install github.com/magefile/mage@latest
//	mage -l          # list targets
//	mage build       # compile for current platform
//	mage test        # run tests
//	mage lint        # run golangci-lint
package main

import (
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
	gomagex "github.com/nirantaraai/nava/mage/golang"
	helmmagex "github.com/nirantaraai/nava/mage/helm"
	k3dmagex "github.com/nirantaraai/nava/mage/k3d"
	operatormagex "github.com/nirantaraai/nava/mage/operator"
)

func init() {
	_ = gomagex.LoadConfig("go.yaml")
	_ = helmmagex.LoadConfig("helm.yaml")
	_ = k3dmagex.LoadConfig("k3d.yaml")
}

// Build compiles the operator binary (config: go.yaml → build).
func Build() error { return gomagex.Build() }

// Test runs the unit test suite (config: go.yaml → test).
func Test() error { return gomagex.Test() }

// Lint runs golangci-lint (config: go.yaml → lint).
func Lint() error { return gomagex.Lint() }

// Vet runs go vet (config: go.yaml → vet).
func Vet() error { return gomagex.Vet() }

// Setup downloads Go modules (config: go.yaml → setup).
func Setup() error { return gomagex.Setup() }

// Race runs tests with race detection (config: go.yaml → race).
func Race() error { return gomagex.Race() }

// Coverage runs tests with coverage profiling (config: go.yaml → coverage).
func Coverage() error { return gomagex.Coverage() }

// Bench runs benchmarks (config: go.yaml → bench).
func Bench() error { return gomagex.Bench() }

// Govulncheck runs govulncheck for vulnerability scanning.
func Govulncheck() error { return gomagex.Govulncheck() }

// IntegrationTest runs integration tests (config: go.yaml → integration).
func IntegrationTest() error { return gomagex.Integration() }

// BuildLinux cross-compiles for linux/amd64 and linux/arm64 (config: go.yaml → crossBuild).
func BuildLinux() error { return gomagex.CrossBuild() }

// Generate runs all code generation (DeepCopy + CRD/RBAC manifests) and syncs CRDs to Helm.
func Generate() error {
	if err := operatormagex.GenerateCode(""); err != nil {
		return err
	}
	return operatormagex.GenerateAndSync("charts/omniroute-operator/crds")
}

// GenerateManifests generates CRD and RBAC manifests using controller-gen.
func GenerateManifests() error {
	return operatormagex.GenerateAndSync("charts/omniroute-operator/crds")
}

// GenerateDeepCopy generates DeepCopy implementations for API types.
func GenerateDeepCopy() error { return operatormagex.GenerateCode("") }

// HelmLint runs helm lint on the operator chart (config: helm.yaml → lint).
func HelmLint() error { return helmmagex.Lint() }

// HelmPackage packages the operator Helm chart (config: helm.yaml → package).
func HelmPackage() error { return helmmagex.Package() }

// HelmPush packages and pushes the Helm chart to the OCI registry (config: helm.yaml → push).
func HelmPush() error { return helmmagex.Push() }

// K3d namespace for local cluster operations (config: k3d.yaml).
type K3d mg.Namespace

// Up creates a k3d cluster, bootstraps infrastructure, and installs the operator.
func (K3d) Up() error { return k3dmagex.Up() }

// Down tears down the k3d cluster.
func (K3d) Down() error { return k3dmagex.Down() }

// Create creates just the k3d cluster.
func (K3d) Create() error { return k3dmagex.ClusterCreate() }

// Delete deletes just the k3d cluster.
func (K3d) Delete() error { return k3dmagex.ClusterDelete() }

// Bootstrap applies infrastructure manifests (namespaces, etc.).
func (K3d) Bootstrap() error { return k3dmagex.Bootstrap() }

// Status shows the cluster state.
func (K3d) Status() error { return k3dmagex.Status() }

// Clean removes build artefacts.
func Clean() error {
	fmt.Println("cleaning build artifacts")
	if err := os.Remove("bin/manager"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll("dist")
}
