//go:build spawnfargate

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

// This file uses go tags to produce a binary that calls the agent spawner function directly for manual testing.
// Build with:
//   go build -tags spawnfargate -o /tmp/spawnfargate-smoke

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	commands, seenCommands := startSmokeDatabaseCommandWorker(ctx)
	agentJobID := getenvInt64Default("SMOKE_AGENT_JOB_ID", 900000001)
	messageID := getenvInt64Default("SMOKE_MESSAGE_ID", 900000001)

	agentJob := DatabaseAgentJobInfo{
		ID:        agentJobID,
		AgentName: defaultAgentJobName,
		Status:    AgentJobStatusCreated,
	}
	message := DatabaseSQSMessageInfo{
		ID:          messageID,
		RawBody:     getenvDefault("SMOKE_RAW_ALERT", "spawnfargate smoke test"),
		MessageType: getenvDefault("SMOKE_MESSAGE_TYPE", "smoke"),
	}

	fmt.Printf("spawnfargate smoke: spawning agentJob=%d agentName=%s\n", agentJob.ID, agentJob.AgentName)
	if !spawnAndTrackAgentJob(ctx, commands, agentJob, message) {
		fmt.Fprintln(os.Stderr, "spawnfargate smoke: spawn chain returned false")
		os.Exit(1)
	}
	cancel()

	command := <-seenCommands
	switch command.Kind {
	case DatabaseCommandMarkAgentJobSpawned:
		fmt.Printf("spawnfargate smoke: spawned taskARN=%s\n", command.ECSTaskARN)
	case DatabaseCommandMarkAgentJobSpawnFailed:
		fmt.Fprintf(os.Stderr, "spawnfargate smoke: spawn failed: %s\n", command.FailureReason)
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "spawnfargate smoke: unexpected first database command %q\n", command.Kind)
		os.Exit(1)
	}
}

func startSmokeDatabaseCommandWorker(ctx context.Context) (chan<- DatabaseCommand, <-chan DatabaseCommand) {
	commands := make(chan DatabaseCommand)
	seenCommands := make(chan DatabaseCommand, 10)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case command := <-commands:
				seenCommands <- command
				command.Reply <- DatabaseCommandResult{}
			}
		}
	}()

	return commands, seenCommands
}

func getenvInt64Default(name string, fallback int64) int64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid %s=%q: %v\n", name, value, err)
		os.Exit(1)
	}
	return parsed
}
