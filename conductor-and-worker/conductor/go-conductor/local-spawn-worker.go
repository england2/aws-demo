package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ai--done Local Docker spawning now builds the docker run command directly instead of shelling out to a helper script.
// This file is a local development helper which allows you to spawn a local docker container as your worker instead of a Fargate container.

// launchWorkerProcessTestingDocker adapts the local Docker smoke launcher to the registry's launcher shape.
// The registry calls it after recording the spawn event, while main_real owns choosing this local process path
// instead of the Fargate launcher used by the SQS scheduler flow.
func launchWorkerProcessTestingDocker(ctx context.Context, config workerSpawnConfig) error {
	_, err := execWorkerProcessTestingDocker(config)
	return err
}

func execWorkerProcessTestingDocker(config workerSpawnConfig) (*exec.Cmd, error) {
	// ai--done use inline Docker command args instead of /usr/local/bin/spawn-local-worker-helper.
	cmd := exec.Command("docker", localDockerWorkerRunArgs(config)...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker %q: %w", config.WorkerID, err)
	}

	fmt.Printf("spawned docker worker %s with pid %d\n", config.WorkerID, cmd.Process.Pid)

	return cmd, nil
}

func localDockerWorkerRunArgs(config workerSpawnConfig) []string {
	workerImage := os.Getenv("WORKER_IMAGE")
	if workerImage == "" {
		workerImage = "condtest-worker"
	}

	args := []string{
		"run",
		"--rm",
		"--network", "host",
		"--name", "condtest-worker-" + config.WorkerID,
		"-e", "CONDUCTOR_GRPC_SERVER_ADDR=" + config.ConductorGrpcServerAddr,
		"-e", "WORKER_ID=" + config.WorkerID,
		"-e", "RUN_DIR=" + config.RunDir,
	}
	if openAIAPIKey := os.Getenv("OPENAI_API_KEY"); openAIAPIKey != "" {
		args = append(args, "-e", "OPENAI_API_KEY="+openAIAPIKey)
	}

	return append(args, workerImage)
}
