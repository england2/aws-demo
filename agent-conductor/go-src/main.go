package main

import (
	"context"
	"fmt"

	agentspawners "agent-orchestrator/agent-spawners"
	"agent-orchestrator/agent-spawners/lib"
)

func main() {
	ctx := context.Background()
	messages, pollErrors := StartSQSPoller(ctx)

	fmt.Println("agent-orchestrator running")

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			agentSpawnersDecideRunSQSMessage(message)
		case err, ok := <-pollErrors:
			if !ok {
				return
			}
			fmt.Printf("poll error: %v\n", err)
		}
	}
}

// runs all DecideRunSQSMessage functions in the registered agents against the most recent sqs message.
func agentSpawnersDecideRunSQSMessage(message lib.SQSMessage) {
	matches := []lib.AgentMatch{}

	for _, agent := range agentspawners.All() {
		match, err := agent.DecideRunSQSMessage(message)
		if err != nil {
			fmt.Printf("agent parser error: agent=%s message_id=%s error=%v\n", agent.Name(), message.MessageID, err)
			continue
		}

		if match.JobPriority > 0 {
			matches = append(matches, match)
		}
	}

	switch len(matches) {
	case 0:
		fmt.Printf("no agent matched sqs message: %s\n", message.MessageID)
	case 1:
		fmt.Printf("selected agent: message_id=%s agent=%s priority=%d\n", message.MessageID, matches[0].AgentName, matches[0].JobPriority)
	default:
		fmt.Printf("ambiguous agent match: message_id=%s matches=%d\n", message.MessageID, len(matches))
	}
}
