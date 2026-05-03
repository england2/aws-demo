package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type CodexCLIWrapper struct {
	SessionName string
	WorkingDir  string
	Prompt      string
	OpenAIKey   string
}

type CodexCLIProcess struct {
	Output <-chan string
	Done   <-chan error
}

func StartCodexCLIWrapper(config CodexCLIWrapper) (*CodexCLIProcess, error) {
	if strings.TrimSpace(config.SessionName) == "" {
		config.SessionName = "agent-codex"
	}
	if strings.TrimSpace(config.WorkingDir) == "" {
		config.WorkingDir = "/home/root/work"
	}
	if strings.TrimSpace(config.Prompt) == "" {
		config.Prompt = "read agents.md and carry out the task"
	}

	args := []string{
		"new-session",
		"-d",
		"-s",
		config.SessionName,
		"-P",
		"-F",
		"#{pane_id}",
		"codex",
		"exec",
		"--skip-git-repo-check",
		config.Prompt,
	}

	start := exec.Command("tmux", args...)
	start.Dir = config.WorkingDir
	start.Env = append(os.Environ(), "OPENAI_API_KEY="+config.OpenAIKey)

	paneBytes, err := start.Output()
	if err != nil {
		return nil, fmt.Errorf("start codex tmux session: %w", err)
	}

	paneID := strings.TrimSpace(string(paneBytes))
	if paneID == "" {
		return nil, fmt.Errorf("tmux did not return a pane id")
	}

	pipe := exec.Command("tmux", "pipe-pane", "-o", "-t", paneID, "cat")
	stdout, err := pipe.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create tmux pipe stdout: %w", err)
	}
	stderr, err := pipe.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create tmux pipe stderr: %w", err)
	}
	if err := pipe.Start(); err != nil {
		return nil, fmt.Errorf("start tmux pipe-pane: %w", err)
	}

	output := make(chan string)
	done := make(chan error, 1)

	go func() {
		defer close(output)
		readRawTerminalStream(output, stdout)
	}()

	// start our agent in a goroutine
	go func() {
		errText, _ := io.ReadAll(stderr)
		if err := pipe.Wait(); err != nil {
			if len(errText) > 0 {
				done <- fmt.Errorf("tmux pipe-pane: %w: %s", err, strings.TrimSpace(string(errText)))
			} else {
				done <- fmt.Errorf("tmux pipe-pane: %w", err)
			}
			close(done)
			return
		}
		done <- nil
		close(done)
	}()

	return &CodexCLIProcess{
		Output: output,
		Done:   done,
	}, nil
}

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
