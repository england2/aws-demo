package sharedlib

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ZipDirectory(sourceDir string, destinationZipPath string) error {
	sourceDirInfo, err := os.Stat(sourceDir)
	if err != nil {
		return fmt.Errorf("stat zip source dir: %w", err)
	}
	if !sourceDirInfo.IsDir() {
		return fmt.Errorf("zip source path %q must be a directory", sourceDir)
	}

	destinationZipAbsPath, err := filepath.Abs(destinationZipPath)
	if err != nil {
		return fmt.Errorf("resolve destination zip path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destinationZipAbsPath), 0o755); err != nil {
		return fmt.Errorf("create destination zip parent dir: %w", err)
	}

	zipCommand := exec.Command("zip", "-r", destinationZipAbsPath, ".")
	zipCommand.Dir = sourceDir
	zipCommandOutput, err := zipCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run zip -r for %q: %w\n%s", sourceDir, err, zipCommandOutput)
	}

	return nil
}

func UnzipBytesToDirectory(zipBytes []byte, destinationDir string) error {
	if err := os.RemoveAll(destinationDir); err != nil {
		return fmt.Errorf("remove old zip destination dir: %w", err)
	}
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return fmt.Errorf("create zip destination dir: %w", err)
	}

	destinationDirAbsPath, err := filepath.Abs(destinationDir)
	if err != nil {
		return fmt.Errorf("resolve zip destination dir: %w", err)
	}

	zipBytesPath := destinationDirAbsPath + ".zip"
	if err := os.WriteFile(zipBytesPath, zipBytes, 0o644); err != nil {
		return fmt.Errorf("write zip bytes to file: %w", err)
	}

	unzipCommand := exec.Command("unzip", "-o", zipBytesPath, "-d", destinationDirAbsPath)
	unzipCommandOutput, err := unzipCommand.CombinedOutput()
	if err != nil {
		if removeErr := os.RemoveAll(destinationDirAbsPath); removeErr != nil {
			return fmt.Errorf("remove zip destination dir after failed unzip: %w", removeErr)
		}
		if strings.Contains(string(unzipCommandOutput), "skipped") {
			return fmt.Errorf("zip entry escapes destination dir: unzip rejected archive path\n%s", unzipCommandOutput)
		}
		return fmt.Errorf("run unzip for %q: %w\n%s", zipBytesPath, err, unzipCommandOutput)
	}

	return nil
}
