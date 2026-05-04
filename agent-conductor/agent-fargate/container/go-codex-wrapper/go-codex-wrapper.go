package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentproto"
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
		if os.Args[1] == "--ending" {
			if err := emitEndingToolEvents(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	// gaurd against the agent accidentally spawning another codex-wrapper by calling the binary directly.
	if os.Getenv("START_AGENT_ALLOWED") != "true" {
		fmt.Fprintln(os.Stderr, "Incorrect tool envokation! do `codex-wrapper -- <toolname>`")
		os.Exit(1)
	}

	ctx := context.Background()
	eventEmitter, err := NewAgentEventEmitter(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// setup
	if err := eventEmitter.Send(ctx, agentproto.AgentWrapperStarted, "agent wrapper started"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if err := eventEmitter.Send(ctx, agentproto.AgentSetupStarted, "agent setup started"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	write_start_time()
	OPENAI_API_KEY = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if OPENAI_API_KEY == "" {
		err := fmt.Errorf("OPENAI_API_KEY is required")
		eventEmitter.SendFailure(ctx, agentproto.AgentSetupFailed, err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// hide the start guard from the agent so accidental no-arg tool calls cannot spawn nested agents
	if err := os.Unsetenv("START_AGENT_ALLOWED"); err != nil {
		eventEmitter.SendFailure(ctx, agentproto.AgentSetupFailed, err)
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
		eventEmitter.SendFailure(ctx, agentproto.AgentSetupFailed, err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if err := eventEmitter.Send(ctx, agentproto.CodexStarted, "codex started"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	go func() {
		for chunk := range process.Output {
			fmt.Print(chunk)
		}
	}()

	if err := <-process.Done; err != nil {
		eventEmitter.SendFailure(ctx, agentproto.CodexExited, err)
		eventEmitter.SendFailure(ctx, agentproto.JobFailed, err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if err := eventEmitter.Send(ctx, agentproto.CodexExited, "codex exited"); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func emitEndingToolEvents(ctx context.Context) error {
	eventEmitter, err := NewAgentEventEmitter(ctx)
	if err != nil {
		return err
	}

	if err := eventEmitter.Send(ctx, agentproto.AgentReportedSuccess, "agent reported success via ending tool"); err != nil {
		return err
	}

	return eventEmitter.Send(ctx, agentproto.JobCompleted, "agent job completed via ending tool")
}
