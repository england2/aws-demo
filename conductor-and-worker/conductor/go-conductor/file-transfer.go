package main

import (
	"fmt"
	"os"
	"path/filepath"

	sharedproto "conductor-testing/proto"
	sharedlib "conductor-testing/sharedlib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// =============================================================
// gRPC Procedure Implementation
// =============================================================

func (s *conductorServer) WorkerRequestsWorkFiles(
	req *sharedproto.FileTransferRequest,
	stream sharedproto.WorkerEventReceiverService_WorkerRequestsWorkFilesServer,
) error {
	workerID, err := workerIDFromIdentity(req.GetWorker())
	if err != nil {
		return err
	}

	worker, ok := s.registry.getWorker(workerID)
	if !ok {
		return status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
	}
	if !worker.didHandshakeSucceed() {
		return status.Errorf(codes.FailedPrecondition, "worker %q has not completed handshake", workerID)
	}

	printWorkerMessage(worker.ID, req.GetWorkerMessage())

	workerZipFilePath, err := prepareWorkerWorkFilesZip(worker.RunDir, workerID, worker.WorkFilesDir)
	if err != nil {
		return fmt.Errorf("prepare worker work files zip: %w", err)
	}

	if err := streamWorkFilesZip(workerZipFilePath, stream); err != nil {
		return err
	}

	worker.recordEvent(WorkerEventGotFilesSucceeded, req)

	return nil
}

func (s *conductorServer) WorkerUploadsFiles(
	stream sharedproto.WorkerEventReceiverService_WorkerUploadsFilesServer,
) error {
	var worker *spawnedWorker
	var workerID string
	var workerUploadMessage string

	uploadedWorkerZipBytes, err := sharedlib.ReceiveFileTransferChunks(func() (sharedlib.FileTransferChunk, error) {
		uploadedFilesChunk, err := stream.Recv()
		if err != nil {
			return sharedlib.FileTransferChunk{}, err
		}

		if worker == nil {
			workerID, err = workerIDFromIdentity(uploadedFilesChunk.GetWorker())
			if err != nil {
				return sharedlib.FileTransferChunk{}, err
			}

			foundWorker, ok := s.registry.getWorker(workerID)
			if !ok {
				return sharedlib.FileTransferChunk{}, status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
			}
			if !foundWorker.didHandshakeSucceed() {
				return sharedlib.FileTransferChunk{}, status.Errorf(codes.FailedPrecondition, "worker %q has not completed handshake", workerID)
			}

			worker = foundWorker
			workerUploadMessage = uploadedFilesChunk.GetWorkerMessage()
			printWorkerMessage(worker.ID, workerUploadMessage)
		}

		return sharedlib.FileTransferChunk{
			Content:    uploadedFilesChunk.GetContent(),
			ChunkIndex: uploadedFilesChunk.GetChunkIndex(),
			FinalChunk: uploadedFilesChunk.GetFinalChunk(),
		}, nil
	})
	if err != nil {
		return err
	}

	if err := sharedlib.UnzipBytesToDirectory(uploadedWorkerZipBytes, uploadedWorkerFilesExtractionDir(worker.RunDir, workerID)); err != nil {
		return fmt.Errorf("unzip uploaded worker files: %w", err)
	}

	worker.recordEvent(WorkerEventUploadedFilesSucceeded, workerUploadMessage)

	return stream.SendAndClose(&sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", workerUploadMessage),
	})
}

// =============================================================
// Serverside Helpers
// =============================================================

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
