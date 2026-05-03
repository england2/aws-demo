package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func write_start_time() {
	metaDir := "/tmp/agent-meta"
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "create agent meta dir: %v\n", err)
		return
	}

	startTime := strconv.FormatInt(time.Now().Unix(), 10)
	startTimePath := filepath.Join(metaDir, "start-time.txt")
	if err := os.WriteFile(startTimePath, []byte(startTime+"\n"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write start time: %v\n", err)
	}
}

var OPENAI_API_KEY string

func main() {
	registerBuiltinTools()
	if len(os.Args) > 1 && runToolArgument(os.Args[1]) {
		return
	}

	// gaurd against the agent accidentally spawning another codex-wrapper by calling the binary directly.
	if os.Getenv("START_AGENT_ALLOWED") != "true" {
		fmt.Fprintln(os.Stderr, "Incorrect tool envokation! do `codex-wrapper -- <toolname>`")
		os.Exit(1)
	}

	// setup
	write_start_time()
	OPENAI_API_KEY = get_openai_key()
	if err := codex_login(OPENAI_API_KEY); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
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
