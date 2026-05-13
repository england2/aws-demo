//go:build spawnfargate

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"agent-orchestrator/fargate"
)

// main is the build-tagged smoke entrypoint for spawning one task without the SQS poller.
// It fabricates the same minimal PolledSQSMessage fields that HandleSQSMessage uses, then sends the derived prompt
// and static adhoc config directly into fargate.Spawn.
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	message := PolledSQSMessage{
		ExternalMessageID: getenvDefault("SMOKE_MESSAGE_ID", "spawnfargate-smoke"),
		Body:              getenvDefault("SMOKE_RAW_ALERT", "spawnfargate smoke test"),
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
