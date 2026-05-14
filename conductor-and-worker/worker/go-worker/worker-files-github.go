package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type WorkerRuntimePaths struct {
	WorkDir          string
	RepoRootDir      string
	AgentMetaDir     string
	JobSuccessPath   string
	EndingReportPath string
	PRMessagePath    string
}

type WorkerArtifactValidationResult struct {
	RepoPath string
	Errors   []string
}

type PullRequestCreationResult struct {
	BranchName string
	Output     string
}

type GitHubIssueCreationResult struct {
	Output string
}

func defaultWorkerRuntimePaths() WorkerRuntimePaths {
	workerAgentMetaDir := filepath.Join(workerWorkFilesDestinationDir, "agent-meta")

	return WorkerRuntimePaths{
		WorkDir:          workerWorkFilesDestinationDir,
		RepoRootDir:      filepath.Join(workerWorkFilesDestinationDir, "repo"),
		AgentMetaDir:     workerAgentMetaDir,
		JobSuccessPath:   filepath.Join(workerAgentMetaDir, "WAS_JOB_SUCCESSFUL"),
		EndingReportPath: filepath.Join(workerAgentMetaDir, "ending-report.md"),
		PRMessagePath:    filepath.Join(workerAgentMetaDir, "pr-message.md"),
	}
}

func ensureDirsExist(dirPaths []string) error {
	for _, dirPath := range dirPaths {
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dirPath, err)
		}
	}

	return nil
}

func readWorkerJobSuccessMarker(workerRuntimePaths WorkerRuntimePaths) (bool, error) {
	workerJobSuccessBytes, err := os.ReadFile(workerRuntimePaths.JobSuccessPath)
	if err != nil {
		return false, fmt.Errorf("read worker success marker %q: %w", workerRuntimePaths.JobSuccessPath, err)
	}

	workerJobSuccessText := strings.TrimSpace(string(workerJobSuccessBytes))
	if workerJobSuccessText == "true" {
		return true, nil
	}
	if workerJobSuccessText == "false" {
		return false, nil
	}
	return false, fmt.Errorf("parse worker success marker %q value %q: expected exactly true or false", workerRuntimePaths.JobSuccessPath, workerJobSuccessText)
}

func validateSuccessfulWorkerArtifacts(workerRuntimePaths WorkerRuntimePaths) WorkerArtifactValidationResult {
	validationResult := WorkerArtifactValidationResult{}

	repoPath, err := findSingleWorkerGitRepo(workerRuntimePaths.RepoRootDir)
	if err != nil {
		validationResult.Errors = append(validationResult.Errors, err.Error())
	} else {
		validationResult.RepoPath = repoPath
		validationResult.Errors = append(validationResult.Errors, validateSuccessfulWorkerGitRepo(repoPath)...)
	}

	if err := validateEndingReportFile(workerRuntimePaths.EndingReportPath); err != nil {
		validationResult.Errors = append(validationResult.Errors, err.Error())
	}

	return validationResult
}

func findSingleWorkerGitRepo(repoRootDir string) (string, error) {
	repoRootEntries, err := os.ReadDir(repoRootDir)
	if err != nil {
		return "", fmt.Errorf("read worker repo root %q: %w", repoRootDir, err)
	}

	gitRepoPaths := []string{}
	for _, repoRootEntry := range repoRootEntries {
		if !repoRootEntry.IsDir() {
			continue
		}

		candidateRepoPath := filepath.Join(repoRootDir, repoRootEntry.Name())
		if isGitRepositoryDirectory(candidateRepoPath) {
			gitRepoPaths = append(gitRepoPaths, candidateRepoPath)
		}
	}

	if len(gitRepoPaths) == 0 {
		return "", fmt.Errorf("expected exactly one Git repo under %q, found none", repoRootDir)
	}
	if len(gitRepoPaths) > 1 {
		return "", fmt.Errorf("expected exactly one Git repo under %q, found %d", repoRootDir, len(gitRepoPaths))
	}

	return gitRepoPaths[0], nil
}

func findOptionalWorkerGitRepo(workerRuntimePaths WorkerRuntimePaths) (string, bool) {
	repoPath, err := findSingleWorkerGitRepo(workerRuntimePaths.RepoRootDir)
	if err != nil {
		fmt.Printf("[internal]: skipping GitHub issue creation because worker repo is unavailable: %v\n", err)
		return "", false
	}

	return repoPath, true
}

func isGitRepositoryDirectory(candidateRepoPath string) bool {
	gitCommandOutput, err := runWorkerRepoCommand(context.Background(), candidateRepoPath, "git", "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(gitCommandOutput)) == "true"
}

func validateSuccessfulWorkerGitRepo(repoPath string) []string {
	validationErrors := []string{}

	branchName, err := currentWorkerRepoBranchName(repoPath)
	if err != nil {
		return append(validationErrors, err.Error())
	}
	if branchName == "main" || branchName == "master" {
		validationErrors = append(validationErrors, fmt.Sprintf("repo %q must be on a feature branch, not %q", repoPath, branchName))
	}
	if err := validateWorkerRepoCleanWorktree(repoPath); err != nil {
		validationErrors = append(validationErrors, err.Error())
	}

	hasBranchDelta, err := workerRepoHasCommittedDeltaFromMain(repoPath)
	if err != nil {
		validationErrors = append(validationErrors, err.Error())
	} else if !hasBranchDelta {
		validationErrors = append(validationErrors, fmt.Sprintf("repo %q has no committed branch delta relative to main", repoPath))
	}

	return validationErrors
}

func validateWorkerRepoCleanWorktree(repoPath string) error {
	gitStatusBytes, err := runWorkerRepoCommand(context.Background(), repoPath, "git", "status", "--porcelain")
	if err != nil {
		return err
	}

	gitStatusText := strings.TrimSpace(string(gitStatusBytes))
	if gitStatusText != "" {
		return fmt.Errorf("repo %q has uncommitted worktree changes:\n%s", repoPath, gitStatusText)
	}

	return nil
}

func validateEndingReportFile(endingReportPath string) error {
	endingReportBytes, err := os.ReadFile(endingReportPath)
	if err != nil {
		return fmt.Errorf("read ending report %q: %w", endingReportPath, err)
	}
	if strings.TrimSpace(string(endingReportBytes)) == "" {
		return fmt.Errorf("ending report %q is empty", endingReportPath)
	}

	return nil
}

func currentWorkerRepoBranchName(repoPath string) (string, error) {
	branchNameBytes, err := runWorkerRepoCommand(context.Background(), repoPath, "git", "branch", "--show-current")
	if err != nil {
		return "", err
	}

	branchName := strings.TrimSpace(string(branchNameBytes))
	if branchName == "" {
		return "", fmt.Errorf("repo %q is not on a named branch", repoPath)
	}

	return branchName, nil
}

func workerRepoHasCommittedDeltaFromMain(repoPath string) (bool, error) {
	baseRef, err := firstExistingWorkerRepoBaseRef(repoPath, []string{"origin/main", "main"})
	if err != nil {
		return false, err
	}

	_, err = runWorkerRepoCommand(context.Background(), repoPath, "git", "diff", "--quiet", baseRef+"...HEAD")
	if err == nil {
		return false, nil
	}

	var commandExitError *exec.ExitError
	if errors.As(err, &commandExitError) && commandExitError.ExitCode() == 1 {
		return true, nil
	}

	return false, err
}

func firstExistingWorkerRepoBaseRef(repoPath string, baseRefs []string) (string, error) {
	for _, baseRef := range baseRefs {
		if _, err := runWorkerRepoCommand(context.Background(), repoPath, "git", "rev-parse", "--verify", baseRef); err == nil {
			return baseRef, nil
		}
	}

	return "", fmt.Errorf("repo %q has neither origin/main nor main available for branch comparison", repoPath)
}

func writeGitHubReportMarkdown(workerRuntimePaths WorkerRuntimePaths, transcriptJSON []byte) (string, error) {
	fmt.Printf("[internal]: building GitHub report from ending_report=%s transcript_bytes=%d\n", workerRuntimePaths.EndingReportPath, len(transcriptJSON))

	endingReportBytes, err := os.ReadFile(workerRuntimePaths.EndingReportPath)
	if err != nil {
		return "", fmt.Errorf("read ending report for GitHub body: %w", err)
	}

	endingReportMarkdown := stripLeadingFinalReportHeading(strings.TrimSpace(string(endingReportBytes)))

	gitHubReportMarkdown := fmt.Sprintf(`# Agent Work Report

## Final Report

%s

## Full Agent Transcript

<details>
<summary>Click to see full agent transcript JSON</summary>

`+"```"+`json
%s
`+"```"+`

</details>
`, endingReportMarkdown, string(transcriptJSON))

	if err := os.WriteFile(workerRuntimePaths.PRMessagePath, []byte(gitHubReportMarkdown), 0o644); err != nil {
		return "", fmt.Errorf("write GitHub report markdown: %w", err)
	}

	return workerRuntimePaths.PRMessagePath, nil
}

func stripLeadingFinalReportHeading(reportMarkdown string) string {
	reportLines := strings.Split(reportMarkdown, "\n")
	if len(reportLines) == 0 {
		return reportMarkdown
	}

	firstLine := strings.TrimSpace(reportLines[0])
	headingText := strings.TrimSpace(strings.TrimLeft(firstLine, "#"))
	if strings.HasPrefix(firstLine, "#") && headingText == "Final Report" {
		return strings.TrimSpace(strings.Join(reportLines[1:], "\n"))
	}

	return reportMarkdown
}

func createPullRequestFromWorkerRepo(
	ctx context.Context,
	repoPath string,
	prMessagePath string,
	workerID string,
) (PullRequestCreationResult, error) {
	branchName, err := currentWorkerRepoBranchName(repoPath)
	if err != nil {
		return PullRequestCreationResult{}, err
	}

	fmt.Printf("[internal %s]: pushing worker repo branch repo=%s branch=%s\n", workerID, repoPath, branchName)
	if _, err := runWorkerRepoCommand(ctx, repoPath, "git", "push", "-u", "origin", branchName); err != nil {
		return PullRequestCreationResult{}, err
	}

	pullRequestTitle := buildGitHubTitleFromReport(prMessagePath, fmt.Sprintf("Worker %s changes", workerID))
	fmt.Printf("[internal %s]: creating GitHub pull request base=main head=%s title=%q body_file=%s\n", workerID, branchName, pullRequestTitle, prMessagePath)
	pullRequestOutput, err := runWorkerRepoCommand(ctx, repoPath, "gh", "pr", "create", "--base", "main", "--head", branchName, "--title", pullRequestTitle, "--body-file", prMessagePath)
	if err != nil {
		return PullRequestCreationResult{}, err
	}

	return PullRequestCreationResult{
		BranchName: branchName,
		Output:     string(pullRequestOutput),
	}, nil
}

func createFailedWorkerGitHubIssue(
	ctx context.Context,
	repoPath string,
	gitHubReportPath string,
	workerID string,
) (GitHubIssueCreationResult, error) {
	failedWorkerTitle := "[agent-failed] " + buildGitHubTitleFromReport(gitHubReportPath, fmt.Sprintf("Worker %s failed", workerID))
	fmt.Printf("[internal %s]: creating failed-worker GitHub issue repo=%s title=%q body_file=%s\n", workerID, repoPath, failedWorkerTitle, gitHubReportPath)
	gitHubIssueOutput, err := runWorkerRepoCommand(ctx, repoPath, "gh", "issue", "create", "--title", failedWorkerTitle, "--body-file", gitHubReportPath)
	if err != nil {
		return GitHubIssueCreationResult{}, err
	}

	return GitHubIssueCreationResult{
		Output: string(gitHubIssueOutput),
	}, nil
}

func buildGitHubTitleFromReport(reportPath string, fallbackTitle string) string {
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		return fallbackTitle
	}

	for _, reportLine := range strings.Split(string(reportBytes), "\n") {
		reportLine = strings.TrimSpace(reportLine)
		reportLine = strings.TrimLeft(reportLine, "# \t")
		if reportLine == "" || reportLine == "Agent Work Report" || reportLine == "Final Report" {
			continue
		}
		if len(reportLine) > 120 {
			return reportLine[:120]
		}
		return reportLine
	}

	return fallbackTitle
}

func runWorkerRepoCommand(ctx context.Context, repoPath string, commandName string, commandArgs ...string) ([]byte, error) {
	fmt.Printf("[internal]: running repo command in %s: %s %s\n", repoPath, commandName, strings.Join(commandArgs, " "))

	repoCommand := exec.CommandContext(ctx, commandName, commandArgs...)
	repoCommand.Dir = repoPath

	var repoCommandOutput bytes.Buffer
	repoCommand.Stdout = &repoCommandOutput
	repoCommand.Stderr = &repoCommandOutput

	if err := repoCommand.Run(); err != nil {
		return repoCommandOutput.Bytes(), fmt.Errorf("run %s %s in %q: %w\n%s", commandName, strings.Join(commandArgs, " "), repoPath, err, repoCommandOutput.String())
	}

	return repoCommandOutput.Bytes(), nil
}
