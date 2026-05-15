package main

import (
	"context"
	"errors"
	"testing"
)

// counterfactual confirmed
// TestWorkerRegistrySpawnWorkerDelegatesLaunchAfterRegistration verifies spawnWorker only owns conductor-side bookkeeping.
// It checks the worker is registered and the launch function receives the prepared config, while the actual process
// or Fargate spawn remains supplied by main.
func TestWorkerRegistrySpawnWorkerDelegatesLaunchAfterRegistration(t *testing.T) {
	registry := newWorkerRegistry()
	workerSpawnConfigForTest := workerSpawnConfig{
		WorkerID:                "worker-test",
		ConductorGrpcServerAddr: "localhost:50055",
		RunDir:                  t.TempDir(),
	}

	launchCalled := false
	err := registry.spawnWorker(context.Background(), workerSpawnConfigForTest, func(ctx context.Context, launchedWorkerConfig workerSpawnConfig) error {
		launchCalled = true
		if launchedWorkerConfig.WorkerID != workerSpawnConfigForTest.WorkerID {
			t.Fatalf("launched worker ID = %q, want %q", launchedWorkerConfig.WorkerID, workerSpawnConfigForTest.WorkerID)
		}
		if _, ok := registry.getWorker(workerSpawnConfigForTest.WorkerID); !ok {
			t.Fatalf("worker %q was not registered before launch", workerSpawnConfigForTest.WorkerID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("spawnWorker error: %v", err)
	}
	if !launchCalled {
		t.Fatal("launch function was not called")
	}
}

// counterfactual confirmed
// TestWorkerRegistrySpawnWorkerRemovesRegisteredWorkerWhenLaunchFails verifies failed launch attempts
// do not leave a phantom active worker behind. This matters because main logs launch failures and keeps
// running, so the registry must clean up the worker it registered before calling the launcher.
func TestWorkerRegistrySpawnWorkerRemovesRegisteredWorkerWhenLaunchFails(t *testing.T) {
	registry := newWorkerRegistry()
	workerSpawnConfigForTest := workerSpawnConfig{
		WorkerID:                "worker-launch-fails",
		ConductorGrpcServerAddr: "localhost:50055",
		RunDir:                  t.TempDir(),
	}
	expectedLaunchError := errors.New("launcher failed")

	err := registry.spawnWorker(context.Background(), workerSpawnConfigForTest, func(ctx context.Context, launchedWorkerConfig workerSpawnConfig) error {
		if launchedWorkerConfig.WorkerID != workerSpawnConfigForTest.WorkerID {
			t.Fatalf("launched worker ID = %q, want %q", launchedWorkerConfig.WorkerID, workerSpawnConfigForTest.WorkerID)
		}
		if _, ok := registry.getWorker(workerSpawnConfigForTest.WorkerID); !ok {
			t.Fatalf("worker %q was not registered before launch", workerSpawnConfigForTest.WorkerID)
		}
		return expectedLaunchError
	})
	if !errors.Is(err, expectedLaunchError) {
		t.Fatalf("spawnWorker error = %v, want wrapped %v", err, expectedLaunchError)
	}
	if _, ok := registry.getWorker(workerSpawnConfigForTest.WorkerID); ok {
		t.Fatalf("worker %q is still registered after launch failure", workerSpawnConfigForTest.WorkerID)
	}
	if numActiveWorkers := registry.getNumActiveWorkers(); numActiveWorkers != 0 {
		t.Fatalf("active worker count = %d, want 0", numActiveWorkers)
	}
}
