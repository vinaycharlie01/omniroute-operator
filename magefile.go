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

	gomagex "github.com/nirantaraai/nava/mage/golang"
)

func init() {
	_ = gomagex.LoadConfig("go.yaml")
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

// Clean removes build artefacts.
func Clean() error {
	fmt.Println("cleaning bin/manager")
	if err := os.Remove("bin/manager"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
