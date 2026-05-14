package main

import (
	"context"
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
