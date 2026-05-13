package sharedlib

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

type FileTransferChunk struct {
	Content    []byte
	ChunkIndex int64
	FinalChunk bool
}

const FileTransferChunkSizeBytes = 32 * 1024

func StreamFileAsChunks(
	sourceFilePath string,
	chunkSizeBytes int,
	sendFileTransferChunk func(FileTransferChunk) error,
) error {
	sourceFile, err := os.Open(sourceFilePath)
	if err != nil {
		return fmt.Errorf("open file transfer source file: %w", err)
	}

	sourceFileInfo, err := sourceFile.Stat()
	if err != nil {
		if closeErr := sourceFile.Close(); closeErr != nil {
			return fmt.Errorf("close file transfer source file after failed stat: %w", closeErr)
		}
		return fmt.Errorf("stat file transfer source file: %w", err)
	}

	chunkBuffer := make([]byte, chunkSizeBytes)
	chunkIndex := int64(0)
	bytesStreamed := int64(0)
	for {
		bytesRead, readErr := sourceFile.Read(chunkBuffer)
		if bytesRead > 0 {
			bytesStreamed += int64(bytesRead)
			if err := sendFileTransferChunk(FileTransferChunk{
				Content:    append([]byte(nil), chunkBuffer[:bytesRead]...),
				ChunkIndex: chunkIndex,
				FinalChunk: bytesStreamed == sourceFileInfo.Size(),
			}); err != nil {
				if closeErr := sourceFile.Close(); closeErr != nil {
					return fmt.Errorf("close file transfer source file after failed stream send: %w", closeErr)
				}
				return fmt.Errorf("send file transfer chunk: %w", err)
			}
			chunkIndex++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if closeErr := sourceFile.Close(); closeErr != nil {
				return fmt.Errorf("close file transfer source file after failed stream read: %w", closeErr)
			}
			return fmt.Errorf("read file transfer source file: %w", readErr)
		}
	}

	if err := sourceFile.Close(); err != nil {
		return fmt.Errorf("close file transfer source file: %w", err)
	}

	return nil
}

func ReceiveFileTransferChunks(
	receiveFileTransferChunk func() (FileTransferChunk, error),
) ([]byte, error) {
	fileTransferZipBytes := bytes.Buffer{}
	nextExpectedChunkIndex := int64(0)
	receivedFinalChunk := false
	for {
		fileTransferChunk, err := receiveFileTransferChunk()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive file transfer chunk: %w", err)
		}

		if fileTransferChunk.ChunkIndex != nextExpectedChunkIndex {
			return nil, fmt.Errorf("received file transfer chunk index %d, expected %d", fileTransferChunk.ChunkIndex, nextExpectedChunkIndex)
		}
		if receivedFinalChunk {
			return nil, fmt.Errorf("received file transfer chunk %d after final chunk", fileTransferChunk.ChunkIndex)
		}

		if _, err := fileTransferZipBytes.Write(fileTransferChunk.Content); err != nil {
			return nil, fmt.Errorf("buffer file transfer chunk: %w", err)
		}

		receivedFinalChunk = fileTransferChunk.FinalChunk
		nextExpectedChunkIndex++
	}
	if !receivedFinalChunk {
		return nil, fmt.Errorf("file transfer stream ended without final chunk")
	}

	return fileTransferZipBytes.Bytes(), nil
}
