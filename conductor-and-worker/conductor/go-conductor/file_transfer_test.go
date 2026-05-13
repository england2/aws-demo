package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	sharedproto "conductor-testing/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testWorkFilesStream struct {
	grpc.ServerStream
	chunks                 []*sharedproto.FileTransferChunk
	failOnChunkIndex       int64
	shouldFailOnChunkIndex bool
}

func (stream *testWorkFilesStream) Send(chunk *sharedproto.FileTransferChunk) error {
	if stream.shouldFailOnChunkIndex && chunk.GetChunkIndex() == stream.failOnChunkIndex {
		return io.ErrUnexpectedEOF
	}

	stream.chunks = append(stream.chunks, &sharedproto.FileTransferChunk{
		Content:    append([]byte(nil), chunk.GetContent()...),
		ChunkIndex: chunk.GetChunkIndex(),
		FinalChunk: chunk.GetFinalChunk(),
	})

	return nil
}

type testUploadedFilesStream struct {
	grpc.ServerStream
	chunks        []*sharedproto.FileTransferChunk
	nextIndex     int
	closeResponse *sharedproto.GeneralResponse
}

func (stream *testUploadedFilesStream) Recv() (*sharedproto.FileTransferChunk, error) {
	if stream.nextIndex >= len(stream.chunks) {
		return nil, io.EOF
	}

	chunk := stream.chunks[stream.nextIndex]
	stream.nextIndex++
	return chunk, nil
}

func (stream *testUploadedFilesStream) SendAndClose(response *sharedproto.GeneralResponse) error {
	stream.closeResponse = response
	return nil
}

func TestPrepareWorkerWorkFilesZipUsesTicketWorkerResources(t *testing.T) {
	workerZipFilePath, err := prepareWorkerWorkFilesZip(t.TempDir(), "worker-test-ticket")
	if err != nil {
		t.Fatalf("prepare worker work files zip: %v", err)
	}

	zipReader, err := zip.OpenReader(workerZipFilePath)
	if err != nil {
		t.Fatalf("open worker work files zip: %v", err)
	}
	defer zipReader.Close()

	assertZipFilesMatchTicketWorkerResources(t, zipReader.File)
}

func TestMustTicketWorkerResourcesDirExistPanicsWhenCwdDoesNotContainResources(t *testing.T) {
	originalWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get original working dir: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("change working dir: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWorkingDir); err != nil {
			t.Fatalf("restore working dir: %v", err)
		}
	}()

	defer func() {
		if recoveredPanic := recover(); recoveredPanic == nil {
			t.Fatal("mustTicketWorkerResourcesDirExist should panic when cwd lacks worker resources")
		}
	}()

	mustTicketWorkerResourcesDirExist()
}

func TestWorkerRequestsWorkFilesStreamsZipAfterHandshakeAndRecordsEvent(t *testing.T) {
	workerID := "worker-test"
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}
	worker.recordEvent(WorkerEventHandshakeSucceeded, &sharedproto.Handshake{})

	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}
	workFilesStream := &testWorkFilesStream{}

	err = conductorServiceImplementation.WorkerRequestsWorkFiles(&sharedproto.FileTransferRequest{
		Worker: &sharedproto.WorkerIdentity{
			WorkerId: workerID,
		},
		WorkerMessage: "[worker-test]: requesting work files",
	}, workFilesStream)
	if err != nil {
		t.Fatalf("request work files: %v", err)
	}

	if !worker.hasEvent(WorkerEventGotFilesSucceeded) {
		t.Fatalf("worker should record %s after successful file stream", WorkerEventGotFilesSucceeded.String())
	}
	if len(workFilesStream.chunks) == 0 {
		t.Fatal("work file stream should send at least one chunk")
	}

	workFilesZipBytes := bytes.Buffer{}
	for chunkIndex, chunk := range workFilesStream.chunks {
		if chunk.GetChunkIndex() != int64(chunkIndex) {
			t.Fatalf("chunk index = %d, want %d", chunk.GetChunkIndex(), chunkIndex)
		}

		isLastChunk := chunkIndex == len(workFilesStream.chunks)-1
		if chunk.GetFinalChunk() != isLastChunk {
			t.Fatalf("chunk %d final_chunk = %t, want %t", chunkIndex, chunk.GetFinalChunk(), isLastChunk)
		}

		if _, err := workFilesZipBytes.Write(chunk.GetContent()); err != nil {
			t.Fatalf("buffer streamed zip chunk: %v", err)
		}
	}

	zipReader, err := zip.NewReader(bytes.NewReader(workFilesZipBytes.Bytes()), int64(workFilesZipBytes.Len()))
	if err != nil {
		t.Fatalf("open streamed zip bytes: %v", err)
	}

	assertZipFilesMatchTicketWorkerResources(t, zipReader.File)
}

func TestWorkerRequestsWorkFilesRequiresHandshake(t *testing.T) {
	workerID := "worker-test"
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}

	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}
	workFilesStream := &testWorkFilesStream{}

	err = conductorServiceImplementation.WorkerRequestsWorkFiles(&sharedproto.FileTransferRequest{
		Worker: &sharedproto.WorkerIdentity{
			WorkerId: workerID,
		},
		WorkerMessage: "[worker-test]: requesting work files",
	}, workFilesStream)
	if err == nil {
		t.Fatal("request work files should fail before handshake")
	}
	if len(workFilesStream.chunks) != 0 {
		t.Fatalf("work file stream sent %d chunks before handshake, want 0", len(workFilesStream.chunks))
	}
	if worker.hasEvent(WorkerEventGotFilesSucceeded) {
		t.Fatalf("worker should not record %s before handshake", WorkerEventGotFilesSucceeded.String())
	}
}

func TestWorkerRequestsWorkFilesDoesNotRecordSuccessWhenStreamSendFails(t *testing.T) {
	workerID := "worker-test"
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}
	worker.recordEvent(WorkerEventHandshakeSucceeded, &sharedproto.Handshake{})

	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}
	workFilesStream := &testWorkFilesStream{
		failOnChunkIndex:       0,
		shouldFailOnChunkIndex: true,
	}

	err = conductorServiceImplementation.WorkerRequestsWorkFiles(&sharedproto.FileTransferRequest{
		Worker: &sharedproto.WorkerIdentity{
			WorkerId: workerID,
		},
		WorkerMessage: "[worker-test]: requesting work files",
	}, workFilesStream)
	if err == nil {
		t.Fatal("request work files should fail when stream send fails")
	}
	if worker.hasEvent(WorkerEventGotFilesSucceeded) {
		t.Fatalf("worker should not record %s after failed file stream", WorkerEventGotFilesSucceeded.String())
	}
}

func TestWorkerUploadsFilesExtractsZipAndRecordsEvent(t *testing.T) {
	workerID := "worker-test-upload"
	workerRunDir := t.TempDir()
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   workerRunDir,
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}
	worker.recordEvent(WorkerEventHandshakeSucceeded, &sharedproto.Handshake{})

	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}
	uploadedFilesZipBytes := buildTestZipBytes(t, map[string]string{
		"ending-report.md": "done\n",
	})
	uploadedFilesStream := &testUploadedFilesStream{
		chunks: buildTestFileTransferChunks(workerID, "[worker-test-upload]: uploading files", uploadedFilesZipBytes, 11),
	}

	if err := conductorServiceImplementation.WorkerUploadsFiles(uploadedFilesStream); err != nil {
		t.Fatalf("upload files: %v", err)
	}

	if !worker.hasEvent(WorkerEventUploadedFilesSucceeded) {
		t.Fatalf("worker should record %s after successful upload", WorkerEventUploadedFilesSucceeded.String())
	}
	if uploadedFilesStream.closeResponse.GetWorkerId() != workerID {
		t.Fatalf("upload response worker id = %q, want %q", uploadedFilesStream.closeResponse.GetWorkerId(), workerID)
	}
	assertFileContents(t, filepath.Join(uploadedWorkerFilesExtractionDir(workerRunDir, workerID), "ending-report.md"), "done\n")
}

func TestWorkerUploadsFilesRequiresHandshake(t *testing.T) {
	workerID := "worker-test-upload-no-handshake"
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}
	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}
	uploadedFilesStream := &testUploadedFilesStream{
		chunks: buildTestFileTransferChunks(workerID, "[worker-test-upload-no-handshake]: uploading files", buildTestZipBytes(t, map[string]string{
			"ending-report.md": "done\n",
		}), 11),
	}

	err = conductorServiceImplementation.WorkerUploadsFiles(uploadedFilesStream)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("upload files status = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
	if worker.hasEvent(WorkerEventUploadedFilesSucceeded) {
		t.Fatalf("worker should not record %s before handshake", WorkerEventUploadedFilesSucceeded.String())
	}
}

func TestWorkerUploadsFilesRequiresSequentialChunks(t *testing.T) {
	workerID := "worker-test-upload-bad-order"
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}
	worker.recordEvent(WorkerEventHandshakeSucceeded, &sharedproto.Handshake{})

	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}
	uploadedFilesStream := &testUploadedFilesStream{
		chunks: []*sharedproto.FileTransferChunk{
			{
				Worker: &sharedproto.WorkerIdentity{
					WorkerId: workerID,
				},
				WorkerMessage: "[worker-test-upload-bad-order]: uploading files",
				Content:       buildTestZipBytes(t, map[string]string{"ending-report.md": "done\n"}),
				ChunkIndex:    1,
				FinalChunk:    true,
			},
		},
	}

	err = conductorServiceImplementation.WorkerUploadsFiles(uploadedFilesStream)
	if err == nil {
		t.Fatal("upload files should fail when first chunk index is not zero")
	}
	if worker.hasEvent(WorkerEventUploadedFilesSucceeded) {
		t.Fatalf("worker should not record %s after bad chunk order", WorkerEventUploadedFilesSucceeded.String())
	}
}

func TestWorkerSendsCodexErrorRecordsEventAndDeregistersWorker(t *testing.T) {
	workerID := "worker-test-codex-error"
	workerRegistry := newWorkerRegistry()
	worker, err := workerRegistry.registerSpawnedWorker(workerSpawnConfig{
		WorkerID: workerID,
		RunDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("register spawned worker: %v", err)
	}
	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}

	response, err := conductorServiceImplementation.WorkerSendsCodexError(context.Background(), &sharedproto.CodexError{
		WorkerId:      workerID,
		WorkerMessage: "[worker-test-codex-error]: codex error: create codex client: EOF",
	})
	if err != nil {
		t.Fatalf("send codex error: %v", err)
	}

	if response.GetWorkerId() != workerID {
		t.Fatalf("response worker id = %q, want %q", response.GetWorkerId(), workerID)
	}
	if !worker.hasEvent(WorkerEventCodexError) {
		t.Fatalf("worker should record %s after codex error", WorkerEventCodexError.String())
	}
	if _, ok := workerRegistry.getWorker(workerID); ok {
		t.Fatalf("worker %q should be deregistered after codex error", workerID)
	}
	if workerRegistry.getNumActiveWorkers() != 0 {
		t.Fatalf("active worker count = %d, want 0", workerRegistry.getNumActiveWorkers())
	}
}

func TestWorkerSendsCodexErrorRequiresRegisteredWorker(t *testing.T) {
	workerRegistry := newWorkerRegistry()
	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}

	_, err := conductorServiceImplementation.WorkerSendsCodexError(context.Background(), &sharedproto.CodexError{
		WorkerId:      "missing-worker",
		WorkerMessage: "[missing-worker]: codex error: create codex client: EOF",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("send codex error status = %v, want %v", status.Code(err), codes.FailedPrecondition)
	}
}

func TestWorkerSendsCodexErrorRequiresWorkerID(t *testing.T) {
	workerRegistry := newWorkerRegistry()
	conductorServiceImplementation := &conductorServer{
		registry: workerRegistry,
	}

	_, err := conductorServiceImplementation.WorkerSendsCodexError(context.Background(), &sharedproto.CodexError{
		WorkerMessage: "codex error: create codex client: EOF",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("send codex error status = %v, want %v", status.Code(err), codes.InvalidArgument)
	}
}

func assertZipFilesMatchTicketWorkerResources(t *testing.T, zippedFiles []*zip.File) {
	t.Helper()

	expectedFilePaths := []string{
		"AGENTS.md",
		"TASK.md",
	}
	for _, expectedFilePath := range expectedFilePaths {
		assertZipFileMatchesTicketWorkerResource(t, zippedFiles, expectedFilePath)
	}
}

func assertZipFileMatchesTicketWorkerResource(t *testing.T, zippedFiles []*zip.File, expectedFilePath string) {
	t.Helper()

	for _, zippedFile := range zippedFiles {
		if zippedFile.Name != expectedFilePath {
			continue
		}

		zippedFileReader, err := zippedFile.Open()
		if err != nil {
			t.Fatalf("open zipped file %q: %v", expectedFilePath, err)
		}
		actualFileContents, readErr := io.ReadAll(zippedFileReader)
		closeErr := zippedFileReader.Close()
		if readErr != nil {
			t.Fatalf("read zipped file %q: %v", expectedFilePath, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close zipped file %q: %v", expectedFilePath, closeErr)
		}

		expectedFileContents, err := os.ReadFile(filepath.Join(ticketWorkerResourceDir, expectedFilePath))
		if err != nil {
			t.Fatalf("read ticket worker resource %q: %v", expectedFilePath, err)
		}
		if string(actualFileContents) != string(expectedFileContents) {
			t.Fatalf("zipped file %q contents = %q, want %q", expectedFilePath, actualFileContents, expectedFileContents)
		}
		return
	}

	t.Fatalf("worker work files zip should include %q", expectedFilePath)
}

func buildTestFileTransferChunks(workerID string, workerMessage string, fileTransferZipBytes []byte, chunkSize int) []*sharedproto.FileTransferChunk {
	chunks := []*sharedproto.FileTransferChunk{}
	for chunkStart := 0; chunkStart < len(fileTransferZipBytes); chunkStart += chunkSize {
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(fileTransferZipBytes) {
			chunkEnd = len(fileTransferZipBytes)
		}

		fileTransferChunk := &sharedproto.FileTransferChunk{
			Content:    append([]byte(nil), fileTransferZipBytes[chunkStart:chunkEnd]...),
			ChunkIndex: int64(len(chunks)),
			FinalChunk: chunkEnd == len(fileTransferZipBytes),
		}
		if len(chunks) == 0 {
			fileTransferChunk.Worker = &sharedproto.WorkerIdentity{
				WorkerId: workerID,
			}
			fileTransferChunk.WorkerMessage = workerMessage
		}

		chunks = append(chunks, fileTransferChunk)
	}

	return chunks
}

func buildTestZipBytes(t *testing.T, files map[string]string) []byte {
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
