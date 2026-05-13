package sharedlib

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestZipDirectoryCreatesReadableZipWithExpectedFiles(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(sourceDir, ".hidden"), 0o755); err != nil {
		t.Fatalf("create hidden source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "1.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, ".hidden", "2.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write hidden source file: %v", err)
	}

	destinationZipPath := filepath.Join(t.TempDir(), "workfiles.zip")
	if err := ZipDirectory(sourceDir, destinationZipPath); err != nil {
		t.Fatalf("zip directory: %v", err)
	}

	zipReader, err := zip.OpenReader(destinationZipPath)
	if err != nil {
		t.Fatalf("open created zip: %v", err)
	}
	defer zipReader.Close()

	expectedFileNames := []string{
		".hidden/2.txt",
		"1.txt",
	}
	// Counterfactual check performed: changing one expected name makes this test fail.
	actualFileNames := []string{}

	for _, zippedFile := range zipReader.File {
		if filepath.IsAbs(zippedFile.Name) {
			t.Fatalf("zip entry %q should be relative", zippedFile.Name)
		}
		if strings.Contains(zippedFile.Name, "..") {
			t.Fatalf("zip entry %q should not escape the zip root", zippedFile.Name)
		}
		if strings.HasSuffix(zippedFile.Name, "/") {
			continue
		}

		actualFileNames = append(actualFileNames, zippedFile.Name)

		expectedFileContents, err := os.ReadFile(filepath.Join(sourceDir, filepath.FromSlash(zippedFile.Name)))
		if err != nil {
			t.Fatalf("read expected file %q: %v", zippedFile.Name, err)
		}

		zippedFileReader, err := zippedFile.Open()
		if err != nil {
			t.Fatalf("open zipped file %q: %v", zippedFile.Name, err)
		}
		actualFileContents, readErr := io.ReadAll(zippedFileReader)
		closeErr := zippedFileReader.Close()
		if readErr != nil {
			t.Fatalf("read zipped file %q: %v", zippedFile.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close zipped file %q: %v", zippedFile.Name, closeErr)
		}

		if string(actualFileContents) != string(expectedFileContents) {
			t.Fatalf("zipped file %q contents did not match fixture", zippedFile.Name)
		}
	}

	sort.Strings(actualFileNames)
	if !reflect.DeepEqual(actualFileNames, expectedFileNames) {
		t.Fatalf("zipped file names = %v, want %v", actualFileNames, expectedFileNames)
	}
}

func TestUnzipBytesToDirectoryExtractsZipBytes(t *testing.T) {
	destinationDir := filepath.Join(t.TempDir(), "workfiles")
	zipBytes := buildTestZipBytes(t, map[string]string{
		"AGENT.md": "agent notes\n",
		"TASK.md":  "task notes\n",
	})

	if err := UnzipBytesToDirectory(zipBytes, destinationDir); err != nil {
		t.Fatalf("unzip bytes to directory: %v", err)
	}

	writtenZipBytes, err := os.ReadFile(destinationDir + ".zip")
	if err != nil {
		t.Fatalf("read written zip bytes file: %v", err)
	}
	if !bytes.Equal(writtenZipBytes, zipBytes) {
		t.Fatal("written zip bytes file should match received zip bytes")
	}

	assertFileContents(t, filepath.Join(destinationDir, "AGENT.md"), "agent notes\n")
	assertFileContents(t, filepath.Join(destinationDir, "TASK.md"), "task notes\n")
}

func TestUnzipBytesToDirectoryRejectsEscapedZipEntry(t *testing.T) {
	destinationDir := filepath.Join(t.TempDir(), "workfiles")
	zipBytes := buildTestZipBytes(t, map[string]string{
		"../escaped.txt": "escaped\n",
	})

	err := UnzipBytesToDirectory(zipBytes, destinationDir)
	if err == nil {
		t.Fatal("unzip bytes should reject zip entry that escapes destination dir")
	}
	if !strings.Contains(err.Error(), "escapes destination dir") {
		t.Fatalf("unzip bytes error = %q, want escape detail", err)
	}
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
