//go:build spawnfargate

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"agent-orchestrator/fargate"
)

// This file uses go tags to produce a binary that calls the agent spawner function directly for manual testing.
// Build and run with:
//   cd agent-conductor/go-agent-conductor
//   go build -tags spawnfargate -o /tmp/spawnfargate-smoke && /tmp/spawnfargate-smoke

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	message := SQSMessage{
		ExternalMessageID: getenvDefault("SMOKE_MESSAGE_ID", "spawnfargate-smoke"),
		RawBody:           getenvDefault("SMOKE_RAW_ALERT", "spawnfargate smoke test"),
	}

	fmt.Printf("spawnfargate smoke: spawning agentName=%s\n", defaultAgentName)
	result, err := fargate.Spawn(ctx, fargate.SpawnRequest{
		Config: adhocFargateSpawnConfig,
		Environment: fargate.WithDebugSSHEnvironment(map[string]string{
			"AGENT_NAME":   defaultAgentName,
			"AGENT_PROMPT": buildAgentPrompt(message),
		}),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawnfargate smoke: spawn failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("spawnfargate smoke: spawned taskARN=%s\n", result.TaskARN)
}
