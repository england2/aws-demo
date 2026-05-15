package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	sharedproto "conductor-testing/proto"
	sharedlib "conductor-testing/sharedlib"
)

var workerWorkFilesDestinationDir = "/worker/work"

func requestWorkFiles(
	ctx context.Context,
	conductorClient sharedproto.WorkerEventReceiverServiceClient,
	workerIdentity *sharedproto.WorkerIdentity,
) error {
	workerID := workerIdentity.GetWorkerId()
	workFilesStream, err := conductorClient.WorkerRequestsWorkFiles(ctx, &sharedproto.FileTransferRequest{
		Worker:        workerIdentity,
		WorkerMessage: "requesting work files and then working...",
	})
	if err != nil {
		return fmt.Errorf("request work files stream: %w", err)
	}

	workFilesZipBytes, err := sharedlib.ReceiveFileTransferChunks(func() (sharedlib.FileTransferChunk, error) {
		workFilesChunk, err := workFilesStream.Recv()
		if err != nil {
			return sharedlib.FileTransferChunk{}, err
		}

		return sharedlib.FileTransferChunk{
			Content:    workFilesChunk.GetContent(),
			ChunkIndex: workFilesChunk.GetChunkIndex(),
			FinalChunk: workFilesChunk.GetFinalChunk(),
		}, nil
	})
	if err != nil {
		return fmt.Errorf("receive work files: %w", err)
	}

	if err := sharedlib.UnzipBytesToDirectory(workFilesZipBytes, workerWorkFilesDestinationDir); err != nil {
		return fmt.Errorf("unzip work files: %w", err)
	}

	fmt.Printf("[internal %s]: work files extracted to %s\n", workerID, workerWorkFilesDestinationDir)

	return nil
}

func uploadFiles(
	ctx context.Context,
	conductorClient sharedproto.WorkerEventReceiverServiceClient,
	workerIdentity *sharedproto.WorkerIdentity,
) error {
	workerID := workerIdentity.GetWorkerId()
	uploadedFilesZipPath := filepath.Join(os.TempDir(), workerID+"-uploaded-files.zip")
	if err := os.Remove(uploadedFilesZipPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old uploaded files zip: %w", err)
	}
	if err := sharedlib.ZipDirectory(workerWorkFilesDestinationDir, uploadedFilesZipPath); err != nil {
		return fmt.Errorf("zip worker files for upload: %w", err)
	}

	uploadedFilesStream, err := conductorClient.WorkerUploadsFiles(ctx)
	if err != nil {
		return fmt.Errorf("open uploaded files stream: %w", err)
	}

	workerUploadMessage := "uploading files"
	if err := sharedlib.StreamFileAsChunks(uploadedFilesZipPath, sharedlib.FileTransferChunkSizeBytes, func(fileTransferChunk sharedlib.FileTransferChunk) error {
		uploadedFilesChunk := &sharedproto.FileTransferChunk{
			Content:    fileTransferChunk.Content,
			ChunkIndex: fileTransferChunk.ChunkIndex,
			FinalChunk: fileTransferChunk.FinalChunk,
		}
		if fileTransferChunk.ChunkIndex == 0 {
			uploadedFilesChunk.Worker = workerIdentity
			uploadedFilesChunk.WorkerMessage = workerUploadMessage
		}

		return uploadedFilesStream.Send(uploadedFilesChunk)
	}); err != nil {
		return fmt.Errorf("stream uploaded files: %w", err)
	}

	uploadedFilesResponse, err := uploadedFilesStream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("close uploaded files stream: %w", err)
	}

	fmt.Printf("[internal %s]: conductor uploaded files response received: %s\n", workerID, uploadedFilesResponse.GetWorkerMessage())

	return nil
}
