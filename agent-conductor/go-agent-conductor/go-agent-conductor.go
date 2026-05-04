package main

import (
	"context"
	"fmt"
	"os"
)

func main() {

	// fail early if the conductor cannot find or initialize a usable database.
	if err := check_load_db(); err != nil {
		fmt.Fprintf(os.Stderr, "load database: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	databaseCommands, err := StartDatabaseWorker(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start database worker: %v\n", err)
		os.Exit(1)
	}

	messages, errors := StartSQSPoller(ctx)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			result := RecordSQSMessageWithDatabase(ctx, databaseCommands, message)
			if result.Err != nil {
				fmt.Fprintf(os.Stderr, "record sqs message: %v\n", result.Err)
				continue
			}

			fmt.Printf("database decision: reason=%s shouldSpawnAgentJob=%t messageID=%d\n", result.Reason, result.ShouldSpawnAgentJob, result.Message.ID)
			if result.AgentJob != nil {
				fmt.Printf("would spawn agentJob: id=%d agentName=%s spawnSQSMessageID=%d\n", result.AgentJob.ID, result.AgentJob.AgentName, result.AgentJob.SpawnSQSMessageID)
			}

			if result.DeleteSQSMessage {
				if err := DeleteSQSMessage(ctx, message.ReceiptHandle); err != nil {
					fmt.Fprintf(os.Stderr, "delete sqs message: %v\n", err)
				}
			}
		case err, ok := <-errors:
			if !ok {
				return
			}
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
