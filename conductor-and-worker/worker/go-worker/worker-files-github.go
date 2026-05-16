package main

import (
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
	MetaInfoPath     string
	EndingReportPath string
	PRMessagePath    string
	GitHubLinkPath   string
}

type PullRequestCreationResult struct {
	BranchName string
	URL        string
	Output     string
}

type GitHubIssueCreationResult struct {
	URL    string
	Output string
}

type GitHubReportMarkdownResult struct {
	Path  string
	Title string
}

func defaultWorkerRuntimePaths() WorkerRuntimePaths {
	workerAgentMetaDir := filepath.Join(workerWorkFilesDestinationDir, "agent-meta")

	return WorkerRuntimePaths{
		WorkDir:          workerWorkFilesDestinationDir,
		RepoRootDir:      filepath.Join(workerWorkFilesDestinationDir, "repo"),
		AgentMetaDir:     workerAgentMetaDir,
		JobSuccessPath:   filepath.Join(workerAgentMetaDir, "WAS_JOB_SUCCESSFUL"),
		MetaInfoPath:     filepath.Join(workerAgentMetaDir, "meta-info.txt"),
		EndingReportPath: filepath.Join(workerAgentMetaDir, "ending-report.md"),
		PRMessagePath:    filepath.Join(workerAgentMetaDir, "pr-message.md"),
		GitHubLinkPath:   filepath.Join(workerAgentMetaDir, "GHLINK"),
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

func validateSuccessfulWorkerArtifacts(workerRuntimePaths WorkerRuntimePaths) (string, error) {
	var repoPath string
	var validationErrors []error

	foundRepoPath, err := findSingleWorkerGitRepo(workerRuntimePaths.RepoRootDir)
	if err != nil {
		validationErrors = append(validationErrors, err)
	} else {
		repoPath = foundRepoPath
		if err := validateSuccessfulWorkerGitRepo(repoPath); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if err := validateEndingReportFile(workerRuntimePaths.EndingReportPath); err != nil {
		validationErrors = append(validationErrors, err)
	}

	return repoPath, errors.Join(validationErrors...)
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

func validateSuccessfulWorkerGitRepo(repoPath string) error {
	branchName, err := currentWorkerRepoBranchName(repoPath)
	if err != nil {
		return err
	}
	var validationErrors []error
	if branchName == "main" || branchName == "master" {
		validationErrors = append(validationErrors, fmt.Errorf("repo %q must be on a feature branch, not %q", repoPath, branchName))
	}
	if err := validateWorkerRepoCleanWorktree(repoPath); err != nil {
		validationErrors = append(validationErrors, err)
	}

	hasBranchDelta, err := workerRepoHasCommittedDeltaFromMain(repoPath)
	if err != nil {
		validationErrors = append(validationErrors, err)
	} else if !hasBranchDelta {
		validationErrors = append(validationErrors, fmt.Errorf("repo %q has no committed branch delta relative to main", repoPath))
	}

	return errors.Join(validationErrors...)
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

func writeGitHubReportMarkdown(workerRuntimePaths WorkerRuntimePaths, transcriptText string) (GitHubReportMarkdownResult, error) {
	fmt.Printf("[internal]: building GitHub report from ending_report=%s transcript_text_bytes=%d\n", workerRuntimePaths.EndingReportPath, len(transcriptText))

	endingReportBytes, err := os.ReadFile(workerRuntimePaths.EndingReportPath)
	if err != nil {
		return GitHubReportMarkdownResult{}, fmt.Errorf("read ending report for GitHub body: %w", err)
	}

	gitHubTitleBytes, err := exec.Command("goawk", "NR == 1 { print; exit }", workerRuntimePaths.MetaInfoPath).CombinedOutput()
	if err != nil {
		return GitHubReportMarkdownResult{}, fmt.Errorf("read GitHub title from worker meta info %q with goawk: %w\n%s", workerRuntimePaths.MetaInfoPath, err, string(gitHubTitleBytes))
	}
	gitHubTitle := strings.TrimSpace(string(gitHubTitleBytes))
	if gitHubTitle == "" {
		return GitHubReportMarkdownResult{}, fmt.Errorf("GitHub title in worker meta info %q is empty", workerRuntimePaths.MetaInfoPath)
	}
	endingReportMarkdown := strings.TrimSpace(string(endingReportBytes))

	gitHubReportMarkdown := fmt.Sprintf(`# Agent Work Report

%s

## Full Agent Transcript

<details>
<summary>Click to see extracted agent transcript text</summary>

`+"```"+`txt
%s
`+"```"+`

</details>
`, endingReportMarkdown, transcriptText)

	if err := os.WriteFile(workerRuntimePaths.PRMessagePath, []byte(gitHubReportMarkdown), 0o644); err != nil {
		return GitHubReportMarkdownResult{}, fmt.Errorf("write GitHub report markdown: %w", err)
	}

	return GitHubReportMarkdownResult{
		Path:  workerRuntimePaths.PRMessagePath,
		Title: gitHubTitle,
	}, nil
}

func createPullRequestFromWorkerRepo(
	ctx context.Context,
	repoPath string,
	prMessagePath string,
	pullRequestTitle string,
	workerID string,
) (PullRequestCreationResult, error) {
	localBranchName, err := currentWorkerRepoBranchName(repoPath)
	if err != nil {
		return PullRequestCreationResult{}, err
	}
	remoteBranchName := workerRemoteBranchName(workerID, localBranchName)

	fmt.Printf("[internal %s]: pushing worker repo branch repo=%s local_branch=%s remote_branch=%s\n", workerID, repoPath, localBranchName, remoteBranchName)
	if _, err := runWorkerRepoCommand(ctx, repoPath, "git", "push", "-u", "origin", localBranchName+":"+remoteBranchName); err != nil {
		return PullRequestCreationResult{}, err
	}

	fmt.Printf("[internal %s]: creating GitHub pull request base=main head=%s title=%q body_file=%s\n", workerID, remoteBranchName, pullRequestTitle, prMessagePath)
	pullRequestOutput, err := runWorkerRepoCommand(ctx, repoPath, "gh", "pr", "create", "--base", "main", "--head", remoteBranchName, "--title", pullRequestTitle, "--body-file", prMessagePath)
	if err != nil {
		return PullRequestCreationResult{}, err
	}
	pullRequestURL, err := extractGitHubURLFromOutput(string(pullRequestOutput))
	if err != nil {
		return PullRequestCreationResult{}, err
	}

	return PullRequestCreationResult{
		BranchName: remoteBranchName,
		URL:        pullRequestURL,
		Output:     string(pullRequestOutput),
	}, nil
}

func workerRemoteBranchName(workerID string, localBranchName string) string {
	return "worker/" + workerID + "/" + localBranchName
}

func createFailedWorkerGitHubIssue(
	ctx context.Context,
	repoPath string,
	gitHubReportPath string,
	gitHubReportTitle string,
	workerID string,
) (GitHubIssueCreationResult, error) {
	failedWorkerTitle := "[agent-failed] " + gitHubReportTitle
	fmt.Printf("[internal %s]: creating failed-worker GitHub issue repo=%s title=%q body_file=%s\n", workerID, repoPath, failedWorkerTitle, gitHubReportPath)
	gitHubIssueOutput, err := runWorkerRepoCommand(ctx, repoPath, "gh", "issue", "create", "--title", failedWorkerTitle, "--body-file", gitHubReportPath)
	if err != nil {
		return GitHubIssueCreationResult{}, err
	}
	gitHubIssueURL, err := extractGitHubURLFromOutput(string(gitHubIssueOutput))
	if err != nil {
		return GitHubIssueCreationResult{}, err
	}

	return GitHubIssueCreationResult{
		URL:    gitHubIssueURL,
		Output: string(gitHubIssueOutput),
	}, nil
}

func extractGitHubURLFromOutput(gitHubCommandOutput string) (string, error) {
	for _, outputField := range strings.Fields(gitHubCommandOutput) {
		if strings.HasPrefix(outputField, "https://") || strings.HasPrefix(outputField, "http://") {
			return outputField, nil
		}
	}

	return "", fmt.Errorf("GitHub command output did not contain a URL: %q", gitHubCommandOutput)
}

func writeGitHubLinkFile(workerRuntimePaths WorkerRuntimePaths, gitHubURL string) error {
	if strings.TrimSpace(gitHubURL) == "" {
		return fmt.Errorf("GitHub URL is required")
	}

	if err := os.WriteFile(workerRuntimePaths.GitHubLinkPath, []byte(gitHubURL+"\n"), 0o644); err != nil {
		return fmt.Errorf("write GitHub link file: %w", err)
	}

	return nil
}

func runWorkerRepoCommand(ctx context.Context, repoPath string, commandName string, commandArgs ...string) ([]byte, error) {
	fmt.Printf("[internal]: running repo command in %s: %s %s\n", repoPath, commandName, strings.Join(commandArgs, " "))

	repoCommand := exec.CommandContext(ctx, commandName, commandArgs...)
	repoCommand.Dir = repoPath

	repoCommandOutput, err := repoCommand.CombinedOutput()
	if err != nil {
		return repoCommandOutput, fmt.Errorf("run %s %s in %q: %w\n%s", commandName, strings.Join(commandArgs, " "), repoPath, err, repoCommandOutput)
	}

	return repoCommandOutput, nil
}
