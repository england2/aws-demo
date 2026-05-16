package main

import (
	"fmt"
	"os"
	"path/filepath"

	sharedproto "conductor-testing/proto"
	sharedlib "conductor-testing/sharedlib"
)

const (
	// full dir ends up being /conductor/run/filetransfer
	fileTransferDirName          = "filetransfer"
	workerWorkFilesDirName       = "workfiles"
	ticketWorkerResourceDir      = "worker-resources/ticket-worker"
	incidentWorkerResourceDir    = "worker-resources/incident-worker"
	workerTaskFileRelativePath   = "TASK.md"
	workerResourceDirectoryPerms = 0o755
	workerTaskFilePerms          = 0o644
)

func streamWorkFilesZip(
	workerZipFilePath string,
	stream sharedproto.WorkerEventReceiverService_WorkerRequestsWorkFilesServer,
) error {
	return sharedlib.StreamFileAsChunks(workerZipFilePath, sharedlib.FileTransferChunkSizeBytes, func(fileTransferChunk sharedlib.FileTransferChunk) error {
		return stream.Send(&sharedproto.FileTransferChunk{
			Content:    fileTransferChunk.Content,
			ChunkIndex: fileTransferChunk.ChunkIndex,
			FinalChunk: fileTransferChunk.FinalChunk,
		})
	})
}

// prepareWorkerWorkFilesZip compresses the already-seeded work directory for one registered worker.
// WorkerRequestsWorkFiles calls it after handshake validation, so workFilesDir must already point at the
// per-worker directory created before spawn by prepareWorkerSpawnConfig.
func prepareWorkerWorkFilesZip(runDir string, workerID string, workFilesDir string) (string, error) {
	if err := validateWorkerWorkFilesDirExists(workFilesDir); err != nil {
		return "", err
	}
	workerZipFilePath := filepath.Join(runDir, fileTransferDirName, workerID+".zip")
	if err := os.Remove(workerZipFilePath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove old worker zip file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(workerZipFilePath), workerResourceDirectoryPerms); err != nil {
		return "", fmt.Errorf("create worker zip parent dir: %w", err)
	}
	if err := sharedlib.ZipDirectory(workFilesDir, workerZipFilePath); err != nil {
		return "", err
	}

	return workerZipFilePath, nil
}

// uploadedWorkerFilesExtractionDir returns the conductor-side destination for files uploaded by a worker.
// WorkerUploadsFiles uses it after receiving the complete upload zip, keeping returned files under the worker run dir.
func uploadedWorkerFilesExtractionDir(runDir string, workerID string) string {
	return filepath.Join(runDir, fileTransferDirName, workerID+"-results")
}

// =============================================================
// Rig Server-side Worker Directory to Transfer to Worker
// =============================================================

// validateWorkerWorkFilesDirExists checks that a template or prepared worker directory can be zipped.
// It is used both before copying templates and before streaming files to the worker, so missing runtime resources fail clearly.
func validateWorkerWorkFilesDirExists(workerWorkFilesDir string) error {
	if workerWorkFilesDir == "" {
		return fmt.Errorf("worker work files dir is required")
	}
	workerWorkFilesDirInfo, err := os.Stat(workerWorkFilesDir)
	if err != nil {
		return fmt.Errorf("required worker work files dir %q must exist: %w", workerWorkFilesDir, err)
	}
	if !workerWorkFilesDirInfo.IsDir() {
		return fmt.Errorf("required worker work files path %q must be a directory", workerWorkFilesDir)
	}

	return nil
}
