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
	"os/exec"
	"path/filepath"

	gomagex "github.com/nirantaraai/nava/mage/golang"
	k3dmagex "github.com/nirantaraai/nava/mage/k3d"
	"github.com/magefile/mage/mg"
)

func init() {
	_ = gomagex.LoadConfig("go.yaml")
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

// BuildLinux cross-compiles for linux/amd64 and linux/arm64 (config: go.yaml → crossBuild).
func BuildLinux() error { return gomagex.CrossBuild() }

// Generate runs all code generation (DeepCopy + CRD/RBAC manifests).
func Generate() error {
	if err := generateDeepCopy(); err != nil {
		return err
	}
	return generateManifests()
}

// GenerateManifests generates CRD and RBAC manifests using controller-gen.
func GenerateManifests() error { return generateManifests() }

// GenerateDeepCopy generates DeepCopy implementations for API types.
func GenerateDeepCopy() error { return generateDeepCopy() }

// HelmLint runs helm lint on the operator chart.
func HelmLint() error {
	fmt.Println("Running Helm lint...")
	cmd := exec.Command("helm", "lint", "charts/omniroute-operator", "--strict")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// HelmPackage packages the operator Helm chart.
func HelmPackage() error {
	if err := os.MkdirAll(".helm-pkg", 0o755); err != nil {
		return err
	}
	fmt.Println("Packaging Helm chart...")
	cmd := exec.Command("helm", "package", "charts/omniroute-operator", "--destination", ".helm-pkg")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// HelmPush packages and pushes the Helm chart to the OCI registry.
func HelmPush() error {
	if err := HelmPackage(); err != nil {
		return err
	}
	pkgs, err := filepath.Glob(".helm-pkg/omniroute-operator-*.tgz")
	if err != nil || len(pkgs) == 0 {
		return fmt.Errorf("no packaged chart found in .helm-pkg/")
	}
	fmt.Println("Pushing Helm chart to OCI registry...")
	cmd := exec.Command("helm", "push", pkgs[0], "oci://ghcr.io/vinaycharlie01/helm-charts")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

func generateManifests() error {
	controllerGen, err := findControllerGen()
	if err != nil {
		return err
	}
	cmd := exec.Command(controllerGen,
		"rbac:roleName=manager-role",
		"crd",
		"webhook",
		"paths=./...",
		"output:crd:artifacts:config=config/crd/bases",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("Generating CRD and RBAC manifests...")
	if err := cmd.Run(); err != nil {
		return err
	}
	return syncCRDToHelm()
}

func syncCRDToHelm() error {
	src := "config/crd/bases/gateway.omniroute.ai_omniroutes.yaml"
	dst := "charts/omniroute-operator/crds/gateway.omniroute.ai_omniroutes.yaml"
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read CRD: %w", err)
	}
	if err := os.MkdirAll("charts/omniroute-operator/crds", 0o755); err != nil {
		return err
	}
	fmt.Println("Syncing CRD to Helm chart...")
	return os.WriteFile(dst, data, 0o644)
}

func generateDeepCopy() error {
	controllerGen, err := findControllerGen()
	if err != nil {
		return err
	}
	cmd := exec.Command(controllerGen,
		"object",
		"paths=./api/...",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("Generating DeepCopy implementations...")
	return cmd.Run()
}

func findControllerGen() (string, error) {
	local := "bin/controller-gen"
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	path, err := exec.LookPath("controller-gen")
	if err != nil {
		return "", fmt.Errorf("controller-gen not found in bin/ or PATH: %w", err)
	}
	return path, nil
}
