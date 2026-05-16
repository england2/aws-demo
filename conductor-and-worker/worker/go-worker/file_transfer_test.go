package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sharedproto "conductor-testing/proto"
	"google.golang.org/grpc"
)

type testWorkerEventReceiverClient struct {
	receivedFileTransferRequest *sharedproto.FileTransferRequest
	receivedCodexError          *sharedproto.CodexError
	workFilesStream             sharedproto.WorkerEventReceiverService_WorkerRequestsWorkFilesClient
	workFilesRequestErr         error
	uploadedFilesStream         sharedproto.WorkerEventReceiverService_WorkerUploadsFilesClient
	uploadedFilesStreamErr      error
	codexErrorResponse          *sharedproto.GeneralResponse
	codexErrorErr               error
}

func (client *testWorkerEventReceiverClient) WorkerStartsHandshake(context.Context, *sharedproto.Handshake, ...grpc.CallOption) (*sharedproto.HandshakeResponse, error) {
	return nil, nil
}

func (client *testWorkerEventReceiverClient) WorkerStartsShutdown(context.Context, *sharedproto.Shutdown, ...grpc.CallOption) (*sharedproto.GeneralResponse, error) {
	return nil, nil
}

func (client *testWorkerEventReceiverClient) WorkerSendsCodexError(
	ctx context.Context,
	req *sharedproto.CodexError,
	opts ...grpc.CallOption,
) (*sharedproto.GeneralResponse, error) {
	client.receivedCodexError = req
	if client.codexErrorResponse != nil || client.codexErrorErr != nil {
		return client.codexErrorResponse, client.codexErrorErr
	}

	return &sharedproto.GeneralResponse{
		WorkerId:      req.GetWorkerId(),
		WorkerMessage: "codex error recorded",
	}, nil
}

func (client *testWorkerEventReceiverClient) WorkerRequestsWorkFiles(
	ctx context.Context,
	req *sharedproto.FileTransferRequest,
	opts ...grpc.CallOption,
) (sharedproto.WorkerEventReceiverService_WorkerRequestsWorkFilesClient, error) {
	client.receivedFileTransferRequest = req
	return client.workFilesStream, client.workFilesRequestErr
}

func (client *testWorkerEventReceiverClient) WorkerUploadsFiles(
	ctx context.Context,
	opts ...grpc.CallOption,
) (sharedproto.WorkerEventReceiverService_WorkerUploadsFilesClient, error) {
	return client.uploadedFilesStream, client.uploadedFilesStreamErr
}

type testWorkFilesClientStream struct {
	grpc.ClientStream
	chunks    []*sharedproto.FileTransferChunk
	nextIndex int
}

func (stream *testWorkFilesClientStream) Recv() (*sharedproto.FileTransferChunk, error) {
	if stream.nextIndex >= len(stream.chunks) {
		return nil, io.EOF
	}

	chunk := stream.chunks[stream.nextIndex]
	stream.nextIndex++
	return chunk, nil
}

type testUploadedFilesClientStream struct {
	grpc.ClientStream
	chunks   []*sharedproto.FileTransferChunk
	response *sharedproto.GeneralResponse
	sendErr  error
	closeErr error
}

func (stream *testUploadedFilesClientStream) Send(chunk *sharedproto.FileTransferChunk) error {
	if stream.sendErr != nil {
		return stream.sendErr
	}

	stream.chunks = append(stream.chunks, &sharedproto.FileTransferChunk{
		WorkerId:      chunk.GetWorkerId(),
		WorkerMessage: chunk.GetWorkerMessage(),
		Content:       append([]byte(nil), chunk.GetContent()...),
		ChunkIndex:    chunk.GetChunkIndex(),
		FinalChunk:    chunk.GetFinalChunk(),
	})

	return nil
}

func (stream *testUploadedFilesClientStream) CloseAndRecv() (*sharedproto.GeneralResponse, error) {
	if stream.closeErr != nil {
		return nil, stream.closeErr
	}
	if stream.response != nil {
		return stream.response, nil
	}

	return &sharedproto.GeneralResponse{
		WorkerMessage: "uploaded files recorded",
	}, nil
}

func TestRequestWorkFilesExtractsReceivedChunks(t *testing.T) {
	workerID := "worker-test-request"
	temporaryWorkerWorkFilesDestinationDir := t.TempDir()
	originalWorkerWorkFilesDestinationDir := workerWorkFilesDestinationDir
	workerWorkFilesDestinationDir = temporaryWorkerWorkFilesDestinationDir
	t.Cleanup(func() {
		workerWorkFilesDestinationDir = originalWorkerWorkFilesDestinationDir
	})

	workFilesZipBytes := buildTestWorkFilesZipBytes(t, map[string]string{
		"AGENT.md": "agent notes\n",
		"TASK.md":  "task notes\n",
	})
	conductorClient := &testWorkerEventReceiverClient{
		workFilesStream: &testWorkFilesClientStream{
			chunks: buildTestFileTransferChunks(workFilesZipBytes, 13),
		},
	}

	if err := requestWorkFiles(context.Background(), conductorClient, workerID); err != nil {
		t.Fatalf("request work files: %v", err)
	}

	if conductorClient.receivedFileTransferRequest.GetWorkerId() != workerID {
		t.Fatalf("request worker id = %q, want %q", conductorClient.receivedFileTransferRequest.GetWorkerId(), workerID)
	}
	expectedWorkerMessage := "requesting work files and then working..."
	if conductorClient.receivedFileTransferRequest.GetWorkerMessage() != expectedWorkerMessage {
		t.Fatalf("request worker message = %q, want %q", conductorClient.receivedFileTransferRequest.GetWorkerMessage(), expectedWorkerMessage)
	}

	assertFileContents(t, filepath.Join(temporaryWorkerWorkFilesDestinationDir, "AGENT.md"), "agent notes\n")
	assertFileContents(t, filepath.Join(temporaryWorkerWorkFilesDestinationDir, "TASK.md"), "task notes\n")
	if _, err := os.Stat(filepath.Join(temporaryWorkerWorkFilesDestinationDir, workerID)); !os.IsNotExist(err) {
		t.Fatalf("worker-specific subdirectory should not exist at extraction root")
	}
}

func TestRequestWorkFilesRequiresSequentialChunks(t *testing.T) {
	workerID := "worker-test-chunk-order"
	workFilesZipBytes := buildTestWorkFilesZipBytes(t, map[string]string{
		"AGENT.md": "agent notes\n",
	})
	conductorClient := &testWorkerEventReceiverClient{
		workFilesStream: &testWorkFilesClientStream{
			chunks: []*sharedproto.FileTransferChunk{
				{
					Content:    workFilesZipBytes,
					ChunkIndex: 1,
					FinalChunk: true,
				},
			},
		},
	}

	err := requestWorkFiles(context.Background(), conductorClient, workerID)
	if err == nil {
		t.Fatal("request work files should fail when first chunk index is not zero")
	}
	if !strings.Contains(err.Error(), "expected 0") {
		t.Fatalf("request work files error = %q, want expected chunk index detail", err)
	}
}

func TestRequestWorkFilesRequiresFinalChunk(t *testing.T) {
	workerID := "worker-test-final-chunk"
	workFilesZipBytes := buildTestWorkFilesZipBytes(t, map[string]string{
		"AGENT.md": "agent notes\n",
	})
	conductorClient := &testWorkerEventReceiverClient{
		workFilesStream: &testWorkFilesClientStream{
			chunks: []*sharedproto.FileTransferChunk{
				{
					Content:    workFilesZipBytes,
					ChunkIndex: 0,
					FinalChunk: false,
				},
			},
		},
	}

	err := requestWorkFiles(context.Background(), conductorClient, workerID)
	if err == nil {
		t.Fatal("request work files should fail when stream ends without final chunk")
	}
	if !strings.Contains(err.Error(), "without final chunk") {
		t.Fatalf("request work files error = %q, want final chunk detail", err)
	}
}

func TestSendCodexErrorSendsExpectedMessage(t *testing.T) {
	workerID := "worker-test-codex-error"
	conductorClient := &testWorkerEventReceiverClient{}

	err := sendCodexError(context.Background(), conductorClient, workerID, io.ErrUnexpectedEOF)
	if err != nil {
		t.Fatalf("send codex error: %v", err)
	}

	if conductorClient.receivedCodexError.GetWorkerId() != workerID {
		t.Fatalf("codex error worker id = %q, want %q", conductorClient.receivedCodexError.GetWorkerId(), workerID)
	}
	expectedWorkerMessage := "codex error: unexpected EOF"
	if conductorClient.receivedCodexError.GetWorkerMessage() != expectedWorkerMessage {
		t.Fatalf("codex error worker message = %q, want %q", conductorClient.receivedCodexError.GetWorkerMessage(), expectedWorkerMessage)
	}
}

func TestUploadFilesStreamsZipWithWorkerMetadataOnFirstChunk(t *testing.T) {
	workerID := "worker-test-upload"
	temporaryWorkerWorkFilesDestinationDir := t.TempDir()
	originalWorkerWorkFilesDestinationDir := workerWorkFilesDestinationDir
	workerWorkFilesDestinationDir = temporaryWorkerWorkFilesDestinationDir
	t.Cleanup(func() {
		workerWorkFilesDestinationDir = originalWorkerWorkFilesDestinationDir
	})

	if err := os.WriteFile(filepath.Join(temporaryWorkerWorkFilesDestinationDir, "ending-report.md"), []byte("done\n"), 0o644); err != nil {
		t.Fatalf("write worker output file: %v", err)
	}

	uploadedFilesStream := &testUploadedFilesClientStream{}
	conductorClient := &testWorkerEventReceiverClient{
		uploadedFilesStream: uploadedFilesStream,
	}

	if err := uploadFiles(context.Background(), conductorClient, workerID); err != nil {
		t.Fatalf("upload files: %v", err)
	}

	if len(uploadedFilesStream.chunks) == 0 {
		t.Fatal("upload files should send at least one chunk")
	}
	firstChunk := uploadedFilesStream.chunks[0]
	if firstChunk.GetWorkerId() != workerID {
		t.Fatalf("first uploaded chunk worker id = %q, want %q", firstChunk.GetWorkerId(), workerID)
	}
	expectedWorkerMessage := "uploading files"
	if firstChunk.GetWorkerMessage() != expectedWorkerMessage {
		t.Fatalf("first uploaded chunk worker message = %q, want %q", firstChunk.GetWorkerMessage(), expectedWorkerMessage)
	}
	for chunkIndex, chunk := range uploadedFilesStream.chunks {
		if chunk.GetChunkIndex() != int64(chunkIndex) {
			t.Fatalf("uploaded chunk index = %d, want %d", chunk.GetChunkIndex(), chunkIndex)
		}
	}
	lastChunk := uploadedFilesStream.chunks[len(uploadedFilesStream.chunks)-1]
	if !lastChunk.GetFinalChunk() {
		t.Fatal("last uploaded chunk should be final")
	}
}

func buildTestFileTransferChunks(workFilesZipBytes []byte, chunkSize int) []*sharedproto.FileTransferChunk {
	chunks := []*sharedproto.FileTransferChunk{}
	for chunkStart := 0; chunkStart < len(workFilesZipBytes); chunkStart += chunkSize {
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(workFilesZipBytes) {
			chunkEnd = len(workFilesZipBytes)
		}

		chunks = append(chunks, &sharedproto.FileTransferChunk{
			Content:    append([]byte(nil), workFilesZipBytes[chunkStart:chunkEnd]...),
			ChunkIndex: int64(len(chunks)),
			FinalChunk: chunkEnd == len(workFilesZipBytes),
		})
	}

	return chunks
}

func buildTestWorkFilesZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	zipBytes := bytes.Buffer{}
	zipWriter := zip.NewWriter(&zipBytes)
	for fileName, fileContents := range files {
		zipEntryWriter, err := zipWriter.Create(fileName)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", fileName, err)
		}
		if _, err := zipEntryWriter.Write([]byte(fileContents)); err != nil {
			t.Fatalf("write zip entry %q: %v", fileName, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return zipBytes.Bytes()
}

func assertFileContents(t *testing.T, filePath string, expectedFileContents string) {
	t.Helper()

	actualFileContents, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file %q: %v", filePath, err)
	}
	if string(actualFileContents) != expectedFileContents {
		t.Fatalf("file %q contents = %q, want %q", filePath, actualFileContents, expectedFileContents)
	}
}
