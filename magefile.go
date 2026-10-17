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
	"github.com/magefile/mage/mg"
	cimagex "github.com/nirantaraai/nava/mage/ci"
	gomagex "github.com/nirantaraai/nava/mage/golang"
	helmmagex "github.com/nirantaraai/nava/mage/helm"
	k3dmagex "github.com/nirantaraai/nava/mage/k3d"
	operatormagex "github.com/nirantaraai/nava/mage/operator"
)

func init() {
	_ = gomagex.LoadConfig("go.yaml")
	_ = helmmagex.LoadConfig("helm.yaml")
	_ = k3dmagex.LoadConfig("k3d.yaml")
	_ = operatormagex.LoadConfig("operator.yaml")
}

// Build compiles the operator binary (config: go.yaml).
func Build() error { return gomagex.Build() }

// Test runs the unit test suite (config: go.yaml).
func Test() error { return gomagex.Test() }

// Lint runs golangci-lint (config: go.yaml).
func Lint() error { return gomagex.Lint() }

// Vet runs go vet (config: go.yaml).
func Vet() error { return gomagex.Vet() }

// Setup downloads Go modules (config: go.yaml).
func Setup() error { return gomagex.Setup() }

// Race runs tests with race detection (config: go.yaml).
func Race() error { return gomagex.Race() }

// Coverage runs tests with coverage profiling (config: go.yaml).
func Coverage() error { return gomagex.Coverage() }

// Bench runs benchmarks (config: go.yaml).
func Bench() error { return gomagex.Bench() }

// Govulncheck runs govulncheck for vulnerability scanning.
func Govulncheck() error { return gomagex.Govulncheck() }

// IntegrationTest runs integration tests (config: go.yaml).
func IntegrationTest() error { return gomagex.Integration() }

// BuildLinux cross-compiles for linux/amd64 and linux/arm64 (config: go.yaml).
func BuildLinux() error { return gomagex.CrossBuild() }

// Generate runs all code generation (DeepCopy + CRD/RBAC manifests) and syncs CRDs to Helm (config: operator.yaml).
func Generate() error { return operatormagex.GenerateFromConfig() }

// GenerateManifests generates CRD and RBAC manifests using controller-gen.
func GenerateManifests() error { return operatormagex.GenerateManifests() }

// GenerateDeepCopy generates DeepCopy implementations for API types.
func GenerateDeepCopy() error { return operatormagex.GenerateCode("") }

// HelmLint runs helm lint on the operator chart (config: helm.yaml).
func HelmLint() error { return helmmagex.Lint() }

// HelmPackage packages the operator Helm chart (config: helm.yaml).
func HelmPackage() error { return helmmagex.Package() }

// HelmPush packages and pushes the Helm chart to the OCI registry (config: helm.yaml).
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

// Clean removes build artefacts (config: operator.yaml).
func Clean() error { return operatormagex.CleanFromConfig() }

// CI namespace for continuous integration targets.
type CI mg.Namespace

// VerifyGenerate runs code generation and fails if the working tree has changes.
func (CI) VerifyGenerate() error { return cimagex.VerifyGeneratedMage("generate") }

// VerifyModTidy runs go mod tidy and fails if go.mod/go.sum change.
func (CI) VerifyModTidy() error { return cimagex.VerifyModTidy() }

// Check runs go vet + mod tidy verification.
func (CI) Check() error { return cimagex.Check("mage") }
