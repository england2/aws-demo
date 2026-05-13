package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// CodexCLIWrapper is the runtime configuration for one Codex CLI process.
// It is populated from Fargate environment variables by main and passed to
// StartCodexCLIWrapper. The OpenAI key is injected only into the child process env.
type CodexCLIWrapper struct {
	SessionName string
	WorkingDir  string
	Prompt      string
	OpenAIKey   string
}

// CodexCLIProcess exposes the spawned Codex process to the outer wrapper.
// Output streams raw stdout/stderr chunks for container logs; Done reports the
// process exit result so main can emit terminal agent events.
type CodexCLIProcess struct {
	Output <-chan string
	Done   <-chan error
}

// StartCodexCLIWrapper starts the headless Codex CLI process for the Fargate task.
// It connects process stdout/stderr to Output and process completion to Done.
// Runtime behavior is intentionally permissive: Codex runs with bypassed sandbox
// approvals because the container is already the isolation boundary for this demo.
func StartCodexCLIWrapper(config CodexCLIWrapper) (*CodexCLIProcess, error) {
	if strings.TrimSpace(config.WorkingDir) == "" {
		config.WorkingDir = "/home/root/work"
	}
	if strings.TrimSpace(config.Prompt) == "" {
		config.Prompt = "read agents.md and carry out the task"
	}

	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--dangerously-bypass-approvals-and-sandbox",
		config.Prompt,
	}

	cmd := exec.Command("codex", args...)
	cmd.Dir = config.WorkingDir
	cmd.Env = append(os.Environ(), "OPENAI_API_KEY="+config.OpenAIKey)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open codex stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}

	output := make(chan string)
	done := make(chan error, 1)

	go func() {
		defer close(output)
		var readers sync.WaitGroup
		readers.Add(2)

		go func() {
			defer readers.Done()
			readRawTerminalStream(output, stdout)
		}()
		go func() {
			defer readers.Done()
			readRawTerminalStream(output, stderr)
		}()

		readers.Wait()
	}()

	go func() {
		defer close(done)
		done <- cmd.Wait()
	}()

	return &CodexCLIProcess{
		Output: output,
		Done:   done,
	}, nil
}

// readRawTerminalStream forwards raw terminal bytes without line buffering.
// Codex emits terminal-oriented output that may not always end with newlines, so
// chunk forwarding preserves live logs better than Scanner-based line reading.
func readRawTerminalStream(output chan<- string, reader io.Reader) {
	buffered := bufio.NewReader(reader)
	buffer := make([]byte, 4096)

	for {
		n, err := buffered.Read(buffer)
		if n > 0 {
			output <- string(buffer[:n])
		}
		if err != nil {
			return
		}
	}
}
