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
// Build and run with:
//   cd agent-conductor/go-agent-conductor
//   go build -tags spawnfargate -o /tmp/spawnfargate-smoke && /tmp/spawnfargate-smoke

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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
	worker, err := spawnFargateAgent(ctx, agentJob, message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawnfargate smoke: spawn failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("spawnfargate smoke: spawned taskARN=%s\n", worker.TaskARN)
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
