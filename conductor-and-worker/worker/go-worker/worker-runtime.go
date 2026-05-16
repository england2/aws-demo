package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	codex "github.com/pmenglund/codex-sdk-go"
	"github.com/pmenglund/codex-sdk-go/protocol"
)

const wrkMaxValidationAttemps = 4

type WorkerCodexRunResult struct {
	ShouldCreatePullRequest bool
	RepoPath                string
}

// Worker Codex turns are run directly in main so the entrypoint shows the full worker order.
// This function only interprets marker/artifact state and returns the next orchestration decision.
func validateWorkerCodexArtifacts(workerRuntimePaths WorkerRuntimePaths) (WorkerCodexRunResult, error) {
	workerJobWasSuccessful, err := readWorkerJobSuccessMarker(workerRuntimePaths)
	if err != nil {
		return WorkerCodexRunResult{}, err
	}
	if !workerJobWasSuccessful {
		return WorkerCodexRunResult{
			ShouldCreatePullRequest: false,
		}, nil
	}

	repoPath, err := validateSuccessfulWorkerArtifacts(workerRuntimePaths)
	if err != nil {
		return WorkerCodexRunResult{}, err
	}

	return WorkerCodexRunResult{
		ShouldCreatePullRequest: true,
		RepoPath:                repoPath,
	}, nil
}

func buildWorkerArtifactValidationFailureError(validationErr error) error {
	return fmt.Errorf("worker artifact validation failed after %d attempts: %s", wrkMaxValidationAttemps, formatValidationErrorForLog(validationErr))
}

func buildWorkerArtifactCorrectionPrompt(
	workerRuntimePaths WorkerRuntimePaths,
	validationErr error,
	attemptNumber int,
	maxAttemptCount int,
) string {
	return fmt.Sprintf(`# Worker Artifact Correction Required

The worker could not continue because required output artifacts are missing or invalid.

This is validation attempt %d of %d.

Fix the exact issues below, then stop. Do not redo unrelated work.

Validation errors:

%s

Required protocol:

- Write %s with exactly true or false.
- Use true only when the repo work is complete, committed, and ready for PR creation.
- Use false when the task is blocked or should not create a PR.
- Place exactly one Git repo under %s.
- When successful, the repo must be on a feature branch with committed changes relative to main.
- Write the final report to %s.
`, attemptNumber, maxAttemptCount, formatValidationErrorForPrompt(validationErr), workerRuntimePaths.JobSuccessPath, workerRuntimePaths.RepoRootDir, workerRuntimePaths.EndingReportPath)
}

func formatValidationErrorForPrompt(validationErr error) string {
	if validationErr == nil {
		return ""
	}

	return "- " + strings.Join(validationErrorMessages(validationErr), "\n- ")
}

func formatValidationErrorForLog(validationErr error) string {
	return strings.Join(validationErrorMessages(validationErr), "; ")
}

func validationErrorMessages(validationErr error) []string {
	type joinedError interface {
		Unwrap() []error
	}

	if joinedValidationErr, ok := validationErr.(joinedError); ok {
		validationErrorMessages := make([]string, 0, len(joinedValidationErr.Unwrap()))
		for _, err := range joinedValidationErr.Unwrap() {
			validationErrorMessages = append(validationErrorMessages, err.Error())
		}
		return validationErrorMessages
	}

	return []string{validationErr.Error()}
}

func readCodexThreadTranscriptText(
	ctx context.Context,
	codexClient *codex.Codex,
	codexThread *codex.Thread,
) (string, error) {
	transcript, err := codexClient.Client().ThreadRead(ctx, protocol.ThreadReadParams{
		ThreadID:     codexThread.ID(),
		IncludeTurns: true,
	})
	if err != nil {
		return "", fmt.Errorf("read codex thread transcript: %w", err)
	}

	transcriptJSON, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal codex thread transcript: %w", err)
	}

	transcriptText, err := extractTranscriptTextFromJSON(transcriptJSON)
	if err != nil {
		return "", fmt.Errorf("extract transcript text: %w", err)
	}

	return transcriptText, nil
}
