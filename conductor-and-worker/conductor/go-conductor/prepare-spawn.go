package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go-conductor/db-internal/shared"
)

// workerSpawnConfig contains all conductor-local data needed to prepare and spawn one worker.
// Main builds it inline from a scheduler decision, prepareWorkerSpawnConfig fills WorkFilesDir, and the registry
// stores the prepared value so WorkerRequestsWorkFiles can later stream the correct per-worker directory.
type workerSpawnConfig struct {
	ScheduleDecision        shared.ScheduleDecision
	ConductorGrpcServerAddr string
	WorkerID                string
	RunDir                  string
	// WorkFilesDir is the prepared server-side directory zipped by WorkerRequestsWorkFiles.
	// It is created before spawn so the worker can request files immediately after handshake.
	WorkFilesDir string
}

// prepareWorkerSpawnConfig seeds server-side files for one scheduler decision and returns the same spawn config shape.
// Main calls it before registry.spawnWorker, which ensures the later WorkerRequestsWorkFiles RPC can stream a stable directory.
func prepareWorkerSpawnConfig(config workerSpawnConfig) (workerSpawnConfig, error) {
	workFilesDir, err := seedWorkerWorkFiles(config)
	if err != nil {
		return workerSpawnConfig{}, err
	}

	config.WorkFilesDir = workFilesDir
	return config, nil
}

// seedWorkerWorkFiles copies the message-type template and writes TASK.md from the scheduler decision text.
// It runs before the worker process starts, so every spawned worker receives files scoped to its worker ID.
func seedWorkerWorkFiles(config workerSpawnConfig) (string, error) {
	workerResourceDir, err := workerResourceDirForScheduleMessageType(config.ScheduleDecision.MessageType)
	if err != nil {
		return "", err
	}
	if err := validateWorkerWorkFilesDirExists(workerResourceDir); err != nil {
		return "", err
	}

	workFilesDir := workerPreparedWorkFilesDir(config.RunDir, config.WorkerID)
	if err := os.RemoveAll(workFilesDir); err != nil {
		return "", fmt.Errorf("remove old worker work files dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(workFilesDir), workerResourceDirectoryPerms); err != nil {
		return "", fmt.Errorf("create worker work files parent dir: %w", err)
	}
	if err := os.CopyFS(workFilesDir, os.DirFS(workerResourceDir)); err != nil {
		return "", fmt.Errorf("copy worker resource dir %q to %q: %w", workerResourceDir, workFilesDir, err)
	}
	if err := os.WriteFile(filepath.Join(workFilesDir, workerTaskFileRelativePath), []byte(config.ScheduleDecision.Text), workerTaskFilePerms); err != nil {
		return "", fmt.Errorf("write worker task file: %w", err)
	}

	return workFilesDir, nil
}

// workerPreparedWorkFilesDir computes the per-worker directory that will later be zipped for transfer.
// The path lives under the conductor run dir so concurrent workers do not mutate shared worker-resources templates.
func workerPreparedWorkFilesDir(runDir string, workerID string) string {
	return filepath.Join(runDir, workerWorkFilesDirName, workerID)
}

// workerResourceDirForScheduleMessageType maps scheduler decision types to static resource templates.
// seedWorkerWorkFiles calls it before copying resources, making unsupported scheduler types fail before spawn.
func workerResourceDirForScheduleMessageType(messageType shared.ScheduleMessageType) (string, error) {
	switch messageType {
	case shared.ScheduleMessageTypeTicket:
		return ticketWorkerResourceDir, nil
	case shared.ScheduleMessageTypeIncident:
		return incidentWorkerResourceDir, nil
	default:
		return "", fmt.Errorf("unsupported schedule message type %q", messageType)
	}
}
