package main

import (
	"context"
	"fmt"
)

// ai--done
// buildFargateWorkerLauncher adapts a prepared Fargate request to the registry launcher boundary.
// Main builds the request before registering the worker, and the returned launcher performs the ECS
// spawn after registry.spawnWorker records the worker identity.
func buildFargateWorkerLauncher(fargateWorkerSpawnRequest FargateWorkerSpawnRequest) workerLauncher {
	return func(spawnContext context.Context, launchedWorkerConfig workerSpawnConfig) error {
		spawnResult, err := Spawn(spawnContext, fargateWorkerSpawnRequest)
		if err != nil {
			return err
		}
		fmt.Printf(
			"[Conductor] spawned fargate worker %s task=%s\n",
			launchedWorkerConfig.WorkerID,
			spawnResult.TaskARN,
		)
		return nil
	}
}

// ai--done
// buildLocalDockerWorkerLauncher adapts the local Docker test spawner to the same registry launcher boundary.
// It can be passed to registry.spawnWorker in the same argument position as the Fargate launcher when main
// needs to run workers on the conductor host for smoke testing.
func buildLocalDockerWorkerLauncher() workerLauncher {
	return func(spawnContext context.Context, launchedWorkerConfig workerSpawnConfig) error {
		return launchWorkerProcessTestingDocker(spawnContext, launchedWorkerConfig)
	}
}
