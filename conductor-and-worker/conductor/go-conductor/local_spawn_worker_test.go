package main

import (
	"reflect"
	"testing"
)

// counter-factural confirmed
func TestLocalDockerWorkerRunArgsMatchFormerHelperScriptShape(t *testing.T) {
	t.Setenv("WORKER_IMAGE", "custom-worker-image")
	t.Setenv("OPENAI_API_KEY", "test-openai-key")

	args := localDockerWorkerRunArgs(workerSpawnConfig{
		ConductorGrpcServerAddr: "localhost:50055",
		WorkerID:                "worker-test",
		RunDir:                  "/tmp/conductor-run",
	})
	expectedArgs := []string{
		"run",
		"--rm",
		"--network", "host",
		"--name", "condtest-worker-worker-test",
		"-e", "CONDUCTOR_GRPC_SERVER_ADDR=localhost:50055",
		"-e", "WORKER_ID=worker-test",
		"-e", "RUN_DIR=/tmp/conductor-run",
		"-e", "OPENAI_API_KEY=test-openai-key",
		"custom-worker-image",
	}
	if !reflect.DeepEqual(args, expectedArgs) {
		t.Fatalf("docker args = %#v, want %#v", args, expectedArgs)
	}
}
