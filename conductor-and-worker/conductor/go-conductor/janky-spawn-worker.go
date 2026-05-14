package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// launchWorkerProcessTestingDocker adapts the local Docker smoke launcher to the registry's launcher shape.
// The registry calls it after recording the spawn event, while main_real owns choosing this local process path
// instead of the Fargate launcher used by the SQS scheduler flow.
func launchWorkerProcessTestingDocker(ctx context.Context, config workerSpawnConfig) error {
	_, err := execWorkerProcessTestingDocker(config)
	return err
}

func execWorkerProcessTestingDocker(config workerSpawnConfig) (*exec.Cmd, error) {
	spawnDockerWorkerScriptPath, err := filepath.Abs("/home/t/spawn-docker-worker-testing")
	if err != nil {
		return nil, fmt.Errorf("resolve docker worker spawn script path: %w", err)
	}

	cmd := exec.Command(spawnDockerWorkerScriptPath)
	cmd.Env = append(
		os.Environ(),
		"CONDUCTOR_GRPC_SERVER_ADDR="+config.ConductorGrpcServerAddr,
		"WORKER_ID="+config.WorkerID,
		"RUN_DIR="+config.RunDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker %q: %w", config.WorkerID, err)
	}

	fmt.Printf("spawned docker worker %s with pid %d\n", config.WorkerID, cmd.Process.Pid)

	return cmd, nil
}
