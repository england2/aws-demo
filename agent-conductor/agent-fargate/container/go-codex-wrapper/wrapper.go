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
