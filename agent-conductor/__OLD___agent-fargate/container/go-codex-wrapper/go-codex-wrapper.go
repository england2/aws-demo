package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// write_start_time stores the wrapper start time for agent-accessible runtime tools.
// The check-time tool reads this file to warn Codex about the Fargate task budget.
// Failures are logged only; missing timing metadata should not stop task startup.
func write_start_time() {
	metaDir := "/tmp/agent-meta"
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create agent meta dir: %v\n", err)
		return
	}

	startTime := strconv.FormatInt(time.Now().Unix(), 10)
	startTimePath := filepath.Join(metaDir, "start-time.txt")
	if err := os.WriteFile(startTimePath, []byte(startTime+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write start time: %v\n", err)
	}
}

var OPENAI_API_KEY string

// main is the Fargate container's wrapper entrypoint.
// It validates runtime env, starts Codex, and streams Codex output to stdout.
func main() {
	// gaurd against the agent accidentally spawning another codex-wrapper by calling the binary directly.
	if os.Getenv("START_AGENT_ALLOWED") != "true" {
		fmt.Fprintln(os.Stderr, "Incorrect wrapper invocation. Use custom-codex-tools for helper tools.")
		os.Exit(1)
	}

	write_start_time()
	OPENAI_API_KEY = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if OPENAI_API_KEY == "" {
		err := fmt.Errorf("OPENAI_API_KEY is required")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// hide the start guard from the agent so accidental no-arg tool calls cannot spawn nested agents
	if err := os.Unsetenv("START_AGENT_ALLOWED"); err != nil {
		fmt.Fprintf(os.Stderr, "unset START_AGENT_ALLOWED: %v\n", err)
		os.Exit(1)
	}

	// start the agent!
	process, err := StartCodexCLIWrapper(CodexCLIWrapper{
		SessionName: "agent-codex",
		WorkingDir:  "/home/root/work",
		Prompt:      strings.TrimSpace(os.Getenv("AGENT_PROMPT")),
		OpenAIKey:   OPENAI_API_KEY,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	go func() {
		for chunk := range process.Output {
			fmt.Print(chunk)
		}
	}()

	if err := <-process.Done; err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
