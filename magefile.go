//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/magefile/mage/mg"
	gomagex "github.com/nirantaraai/nava/mage/golang"
)

func init() {
	_ = gomagex.LoadConfig("go.yaml")
}

// Go namespace for Go development targets.
type Go mg.Namespace

// Build compiles the operator binary.
func (Go) Build() error { return gomagex.Build() }

// Test runs the test suite.
func (Go) Test() error { return gomagex.Test() }

// Race runs tests with race detection.
func (Go) Race() error { return gomagex.Race() }

// Coverage runs tests with coverage profiling.
func (Go) Coverage() error { return gomagex.Coverage() }

// Bench runs benchmarks.
func (Go) Bench() error { return gomagex.Bench() }

// Lint runs golangci-lint.
func (Go) Lint() error { return gomagex.Lint() }

// Vet runs go vet.
func (Go) Vet() error { return gomagex.Vet() }

// Setup downloads Go module dependencies.
func (Go) Setup() error { return gomagex.Setup() }

// Generate namespace for code generation targets.
type Generate mg.Namespace

// Manifests generates CRD and RBAC manifests using controller-gen.
func (Generate) Manifests() error {
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
	return cmd.Run()
}

// DeepCopy generates DeepCopy implementations for API types.
func (Generate) DeepCopy() error {
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

// All runs all code generation (DeepCopy + Manifests).
func (Generate) All() error {
	if err := (Generate{}).DeepCopy(); err != nil {
		return err
	}
	return (Generate{}).Manifests()
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
