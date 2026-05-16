package main

import (
	"context"
	"fmt"

	sharedproto "conductor-testing/proto"
	sharedlib "conductor-testing/sharedlib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The conductor's gRPC server invokes our server-side RPC methods without giving us a
// convenient client identity. Each worker request carries worker_id in the protobuf payload,
// and the registry maps that ID to the conductor's in-memory worker representation (that is, an
// instance the `spawnedWorker` struct).
type conductorServer struct {
	sharedproto.UnimplementedWorkerEventReceiverServiceServer
	registry *workerRegistry
}

func (s *conductorServer) WorkerStartsHandshake(ctx context.Context, msg *sharedproto.Handshake) (*sharedproto.HandshakeResponse, error) {
	workerID := msg.GetWorkerId()
	if err := validateWorkerID(workerID); err != nil {
		return nil, err
	}

	worker, err := s.registry.registerWorkerHandshake(workerID, msg)
	if err != nil {
		return nil, err
	}

	printWorkerMessage(worker.ID, msg.GetWorkerMessage())

	return &sharedproto.HandshakeResponse{
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

func (s *conductorServer) WorkerStartsShutdown(ctx context.Context, msg *sharedproto.Shutdown) (*sharedproto.GeneralResponse, error) {
	workerID := msg.GetWorkerId()
	if err := validateWorkerID(workerID); err != nil {
		return nil, err
	}

	worker, err := s.registry.registerWorkerSafelyEnded(workerID, msg)
	if err != nil {
		return nil, err
	}

	printWorkerMessage(worker.ID, msg.GetWorkerMessage())

	if err := s.registry.waitFargateAndDeregister(workerID); err != nil {
		return nil, err
	}

	return &sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

func (s *conductorServer) WorkerSendsCodexError(ctx context.Context, msg *sharedproto.CodexError) (*sharedproto.GeneralResponse, error) {
	workerID := msg.GetWorkerId()
	if err := validateWorkerID(workerID); err != nil {
		return nil, err
	}

	worker, err := s.registry.recordWorkerErrorAndDeregister(workerID, msg)
	if err != nil {
		return nil, err
	}

	printWorkerMessage(worker.ID, msg.GetWorkerMessage())

	return &sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

func (s *conductorServer) WorkerRequestsWorkFiles(
	req *sharedproto.FileTransferRequest,
	stream sharedproto.WorkerEventReceiverService_WorkerRequestsWorkFilesServer,
) error {
	workerID := req.GetWorkerId()
	if err := validateWorkerID(workerID); err != nil {
		return err
	}

	worker, err := s.registry.getRequiredWorker(workerID)
	if err != nil {
		return err
	}
	if !worker.hasEvent(handshakeSucceeded) {
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

	worker.recordEvent(gotFilesSucceeded, req)

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
			workerID = uploadedFilesChunk.GetWorkerId()
			if err := validateWorkerID(workerID); err != nil {
				return sharedlib.FileTransferChunk{}, err
			}

			foundWorker, err := s.registry.getRequiredWorker(workerID)
			if err != nil {
				return sharedlib.FileTransferChunk{}, err
			}
			if !foundWorker.hasEvent(handshakeSucceeded) {
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

	worker.recordEvent(uploadedFilesSucceeded, workerUploadMessage)

	return stream.SendAndClose(&sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", workerUploadMessage),
	})
}
