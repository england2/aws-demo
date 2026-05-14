package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWorkerJobSuccessMarkerParsesBooleans(t *testing.T) {
	workerRuntimePaths := testWorkerRuntimePaths(t)

	if err := os.WriteFile(workerRuntimePaths.JobSuccessPath, []byte("true\n"), 0o644); err != nil {
		t.Fatalf("write true success marker: %v", err)
	}
	workerJobWasSuccessful, err := readWorkerJobSuccessMarker(workerRuntimePaths)
	if err != nil {
		t.Fatalf("read true success marker: %v", err)
	}
	if !workerJobWasSuccessful {
		t.Fatal("success marker true should parse as successful")
	}

	if err := os.WriteFile(workerRuntimePaths.JobSuccessPath, []byte("false\n"), 0o644); err != nil {
		t.Fatalf("write false success marker: %v", err)
	}
	workerJobWasSuccessful, err = readWorkerJobSuccessMarker(workerRuntimePaths)
	if err != nil {
		t.Fatalf("read false success marker: %v", err)
	}
	if workerJobWasSuccessful {
		t.Fatal("success marker false should parse as unsuccessful")
	}
}

func TestReadWorkerJobSuccessMarkerRejectsMissingAndInvalidMarker(t *testing.T) {
	workerRuntimePaths := testWorkerRuntimePaths(t)

	if _, err := readWorkerJobSuccessMarker(workerRuntimePaths); err == nil {
		t.Fatal("missing success marker should fail")
	}

	if err := os.WriteFile(workerRuntimePaths.JobSuccessPath, []byte("maybe\n"), 0o644); err != nil {
		t.Fatalf("write invalid success marker: %v", err)
	}
	if _, err := readWorkerJobSuccessMarker(workerRuntimePaths); err == nil {
		t.Fatal("invalid success marker should fail")
	}

	if err := os.WriteFile(workerRuntimePaths.JobSuccessPath, []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write non-canonical success marker: %v", err)
	}
	if _, err := readWorkerJobSuccessMarker(workerRuntimePaths); err == nil {
		t.Fatal("non-canonical success marker should fail")
	}
}

func TestValidateSuccessfulWorkerArtifactsAcceptsFeatureBranchWithCommittedDelta(t *testing.T) {
	workerRuntimePaths := testWorkerRuntimePaths(t)
	repoPath := filepath.Join(workerRuntimePaths.RepoRootDir, "example")
	createTestGitRepoWithFeatureBranchCommit(t, repoPath)
	if err := os.WriteFile(workerRuntimePaths.EndingReportPath, []byte("# Final Report\n\nOutcome: Succeeded.\n"), 0o644); err != nil {
		t.Fatalf("write ending report: %v", err)
	}

	validationResult := validateSuccessfulWorkerArtifacts(workerRuntimePaths)
	if len(validationResult.Errors) != 0 {
		t.Fatalf("validation errors = %v, want none", validationResult.Errors)
	}
	if validationResult.RepoPath != repoPath {
		t.Fatalf("repo path = %q, want %q", validationResult.RepoPath, repoPath)
	}
}

func TestValidateSuccessfulWorkerArtifactsRejectsMainBranchWithoutReport(t *testing.T) {
	workerRuntimePaths := testWorkerRuntimePaths(t)
	repoPath := filepath.Join(workerRuntimePaths.RepoRootDir, "example")
	createTestGitRepoOnMain(t, repoPath)

	validationResult := validateSuccessfulWorkerArtifacts(workerRuntimePaths)
	joinedValidationErrors := strings.Join(validationResult.Errors, "\n")
	if !strings.Contains(joinedValidationErrors, "must be on a feature branch") {
		t.Fatalf("validation errors = %q, want feature branch error", joinedValidationErrors)
	}
	if !strings.Contains(joinedValidationErrors, "ending-report.md") {
		t.Fatalf("validation errors = %q, want ending report error", joinedValidationErrors)
	}
}

func TestValidateSuccessfulWorkerArtifactsRejectsUncommittedWorktree(t *testing.T) {
	workerRuntimePaths := testWorkerRuntimePaths(t)
	repoPath := filepath.Join(workerRuntimePaths.RepoRootDir, "example")
	createTestGitRepoWithFeatureBranchCommit(t, repoPath)
	if err := os.WriteFile(filepath.Join(repoPath, "leftover.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatalf("write uncommitted file: %v", err)
	}
	if err := os.WriteFile(workerRuntimePaths.EndingReportPath, []byte("# Final Report\n\nOutcome: Succeeded.\n"), 0o644); err != nil {
		t.Fatalf("write ending report: %v", err)
	}

	validationResult := validateSuccessfulWorkerArtifacts(workerRuntimePaths)
	joinedValidationErrors := strings.Join(validationResult.Errors, "\n")
	if !strings.Contains(joinedValidationErrors, "uncommitted worktree changes") {
		t.Fatalf("validation errors = %q, want uncommitted worktree error", joinedValidationErrors)
	}
}

// counter-factural confirmed
func TestWriteGitHubReportMarkdownIncludesReportAndTranscriptDetails(t *testing.T) {
	workerRuntimePaths := testWorkerRuntimePaths(t)
	reportMarkdown := "Fix number-adder CLI args\n\n# Final Report\n\nOutcome: Succeeded.\n"
	if err := os.WriteFile(workerRuntimePaths.EndingReportPath, []byte(reportMarkdown), 0o644); err != nil {
		t.Fatalf("write ending report: %v", err)
	}

	gitHubReportMarkdownResult, err := writeGitHubReportMarkdown(workerRuntimePaths, []byte(`{"turns":[{"role":"assistant"}]}`))
	if err != nil {
		t.Fatalf("write GitHub report markdown: %v", err)
	}
	if gitHubReportMarkdownResult.Title != "Fix number-adder CLI args" {
		t.Fatalf("GitHub report title = %q, want first ending report line", gitHubReportMarkdownResult.Title)
	}

	gitHubReportBytes, err := os.ReadFile(gitHubReportMarkdownResult.Path)
	if err != nil {
		t.Fatalf("read GitHub report markdown: %v", err)
	}
	gitHubReportText := string(gitHubReportBytes)
	for _, expectedText := range []string{
		"# Agent Work Report",
		"## Final Report",
		"Outcome: Succeeded.",
		"## Full Agent Transcript",
		"<details>",
		"Click to see full agent transcript JSON",
		`{"turns":[{"role":"assistant"}]}`,
	} {
		if !strings.Contains(gitHubReportText, expectedText) {
			t.Fatalf("GitHub report markdown missing %q:\n%s", expectedText, gitHubReportText)
		}
	}
	if strings.Contains(gitHubReportText, "## Final Report\n\n# Final Report") {
		t.Fatalf("GitHub report markdown repeats final report headings:\n%s", gitHubReportText)
	}
	if strings.Contains(gitHubReportText, "Fix number-adder CLI args") {
		t.Fatalf("GitHub report markdown should not render first-line title:\n%s", gitHubReportText)
	}
	if !strings.Contains(gitHubReportText, "Outcome: Succeeded.\n\n## Full Agent Transcript\n\n<details>") {
		t.Fatalf("GitHub transcript details should be under its own heading:\n%s", gitHubReportText)
	}
}

func TestCreatePullRequestFromWorkerRepoUsesGitPushAndGhPRCreate(t *testing.T) {
	temporaryBinDir := t.TempDir()
	commandLogPath := filepath.Join(t.TempDir(), "commands.log")
	writeExecutableScript(t, filepath.Join(temporaryBinDir, "git"), `#!/bin/sh
printf 'git %s\n' "$*" >> "$TEST_COMMAND_LOG"
if [ "$1" = "branch" ] && [ "$2" = "--show-current" ]; then
  printf 'feature/test\n'
  exit 0
fi
if [ "$1" = "push" ]; then
  printf 'pushed\n'
  exit 0
fi
exit 1
`)
	writeExecutableScript(t, filepath.Join(temporaryBinDir, "gh"), `#!/bin/sh
printf 'gh %s\n' "$*" >> "$TEST_COMMAND_LOG"
printf 'https://github.example/pull/1\n'
`)
	t.Setenv("PATH", temporaryBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_COMMAND_LOG", commandLogPath)

	repoPath := t.TempDir()
	prMessagePath := filepath.Join(t.TempDir(), "pr-message.md")
	if err := os.WriteFile(prMessagePath, []byte("# Agent Work Report\n\nFix issue\n"), 0o644); err != nil {
		t.Fatalf("write PR message: %v", err)
	}

	pullRequestCreationResult, err := createPullRequestFromWorkerRepo(context.Background(), repoPath, prMessagePath, "Add number-adder CLI args", "worker-test")
	if err != nil {
		t.Fatalf("create pull request: %v", err)
	}
	if pullRequestCreationResult.BranchName != "feature/test" {
		t.Fatalf("branch name = %q, want feature/test", pullRequestCreationResult.BranchName)
	}

	commandLogBytes, err := os.ReadFile(commandLogPath)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	commandLogText := string(commandLogBytes)
	if !strings.Contains(commandLogText, "git push -u origin feature/test") {
		t.Fatalf("command log missing git push:\n%s", commandLogText)
	}
	if !strings.Contains(commandLogText, "gh pr create --base main --head feature/test") {
		t.Fatalf("command log missing gh pr create:\n%s", commandLogText)
	}
	if !strings.Contains(commandLogText, "--body-file "+prMessagePath) {
		t.Fatalf("command log missing body file path:\n%s", commandLogText)
	}
	if !strings.Contains(commandLogText, "--title Add number-adder CLI args") {
		t.Fatalf("command log missing first-line report title:\n%s", commandLogText)
	}
}

func TestCreateFailedWorkerGitHubIssueUsesPassedReportTitle(t *testing.T) {
	temporaryBinDir := t.TempDir()
	commandLogPath := filepath.Join(t.TempDir(), "commands.log")
	writeExecutableScript(t, filepath.Join(temporaryBinDir, "gh"), `#!/bin/sh
printf 'gh %s\n' "$*" >> "$TEST_COMMAND_LOG"
printf 'https://github.example/issues/1\n'
`)
	t.Setenv("PATH", temporaryBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_COMMAND_LOG", commandLogPath)

	repoPath := t.TempDir()
	gitHubReportPath := filepath.Join(t.TempDir(), "github-report.md")
	if err := os.WriteFile(gitHubReportPath, []byte("# Agent Work Report\n\n## Final Report\n\nBody.\n"), 0o644); err != nil {
		t.Fatalf("write GitHub report: %v", err)
	}

	_, err := createFailedWorkerGitHubIssue(context.Background(), repoPath, gitHubReportPath, "GitHub auth failed during sparse checkout", "worker-test")
	if err != nil {
		t.Fatalf("create failed worker GitHub issue: %v", err)
	}

	commandLogBytes, err := os.ReadFile(commandLogPath)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	commandLogText := string(commandLogBytes)
	if !strings.Contains(commandLogText, "gh issue create --title [agent-failed] GitHub auth failed during sparse checkout") {
		t.Fatalf("command log missing failed issue title:\n%s", commandLogText)
	}
	if strings.Contains(commandLogText, "Agent's Job Understanding") || strings.Contains(commandLogText, "Final Report") {
		t.Fatalf("failed issue title should not be derived from report headings:\n%s", commandLogText)
	}
}

func testWorkerRuntimePaths(t *testing.T) WorkerRuntimePaths {
	t.Helper()

	workDir := t.TempDir()
	workerRuntimePaths := WorkerRuntimePaths{
		WorkDir:          workDir,
		RepoRootDir:      filepath.Join(workDir, "repo"),
		AgentMetaDir:     filepath.Join(workDir, "agent-meta"),
		JobSuccessPath:   filepath.Join(workDir, "agent-meta", "WAS_JOB_SUCCESSFUL"),
		EndingReportPath: filepath.Join(workDir, "agent-meta", "ending-report.md"),
		PRMessagePath:    filepath.Join(workDir, "agent-meta", "pr-message.md"),
	}
	if err := ensureDirsExist([]string{workerRuntimePaths.RepoRootDir, workerRuntimePaths.AgentMetaDir}); err != nil {
		t.Fatalf("create test worker runtime dirs: %v", err)
	}

	return workerRuntimePaths
}

func createTestGitRepoWithFeatureBranchCommit(t *testing.T, repoPath string) {
	t.Helper()

	createTestGitRepoOnMain(t, repoPath)
	runTestCommand(t, repoPath, "git", "checkout", "-b", "feature/test")
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runTestCommand(t, repoPath, "git", "add", ".")
	runTestCommand(t, repoPath, "git", "commit", "-m", "Add feature")
}

func createTestGitRepoOnMain(t *testing.T, repoPath string) {
	t.Helper()

	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("create git repo dir: %v", err)
	}
	runTestCommand(t, repoPath, "git", "init", "-b", "main")
	runTestCommand(t, repoPath, "git", "config", "user.email", "worker-test@example.com")
	runTestCommand(t, repoPath, "git", "config", "user.name", "Worker Test")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("readme\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runTestCommand(t, repoPath, "git", "add", ".")
	runTestCommand(t, repoPath, "git", "commit", "-m", "Initial commit")
}

func runTestCommand(t *testing.T, commandDir string, commandName string, commandArgs ...string) {
	t.Helper()

	testCommand := exec.Command(commandName, commandArgs...)
	testCommand.Dir = commandDir
	testCommandOutput, err := testCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %s: %v\n%s", commandName, strings.Join(commandArgs, " "), err, testCommandOutput)
	}
}

func writeExecutableScript(t *testing.T, scriptPath string, scriptContents string) {
	t.Helper()

	if err := os.WriteFile(scriptPath, []byte(scriptContents), 0o755); err != nil {
		t.Fatalf("write executable script %q: %v", scriptPath, err)
	}
}
