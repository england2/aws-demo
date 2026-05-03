package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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

	fifoPath := tmuxPipeFIFOPath(config.SessionName, paneID)
	_ = os.Remove(fifoPath)
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		return nil, fmt.Errorf("create tmux pipe fifo: %w", err)
	}

	pipe := exec.Command("tmux", "pipe-pane", "-o", "-t", paneID, "cat > "+fifoPath)
	if pipeOutput, err := pipe.CombinedOutput(); err != nil {
		_ = os.Remove(fifoPath)
		return nil, fmt.Errorf("install tmux pipe-pane: %w: %s", err, strings.TrimSpace(string(pipeOutput)))
	}

	fifo, err := os.OpenFile(fifoPath, os.O_RDONLY, 0600)
	if err != nil {
		_ = os.Remove(fifoPath)
		return nil, fmt.Errorf("open tmux pipe fifo: %w", err)
	}

	output := make(chan string)
	done := make(chan error, 1)

	go func() {
		defer close(output)
		defer fifo.Close()
		defer os.Remove(fifoPath)

		readRawTerminalStream(output, fifo)
	}()

	go func() {
		defer close(done)
		done <- waitForTmuxSession(config.SessionName)
	}()

	return &CodexCLIProcess{
		Output: output,
		Done:   done,
	}, nil
}

func waitForTmuxSession(sessionName string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		cmd := exec.Command("tmux", "has-session", "-t", sessionName)
		if err := cmd.Run(); err != nil {
			return nil
		}

		<-ticker.C
	}
}

func tmuxPipeFIFOPath(sessionName string, paneID string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		".", "-",
		"%", "",
	)
	return filepath.Join("/tmp/agent-meta", "tmux-"+replacer.Replace(sessionName)+"-"+replacer.Replace(paneID)+".fifo")
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
