// Package mocks contains generated mock implementations for testing.
package mocks

//go:generate mockgen -destination=client_mock.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client Client,StatusWriter
