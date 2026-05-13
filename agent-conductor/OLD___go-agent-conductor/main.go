//go:build !spawnfargate

package main

import (
	"context"
	"fmt"
	"os"
)

// main wires the conductor runtime from SQS polling into one Fargate spawn per received delivery.
// It creates the poller before starting the receive loop, then passes each parsed message to HandleSQSMessage
// so deletion remains ordered after a successful ECS RunTask call.
func main() {
	fmt.Println("Agent Conductor started!")

	ctx := context.Background()

	poller, err := NewTicketCloudWatchSQSPoller(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start sqs poller: %v\n", err)
		os.Exit(1)
	}
	messages, errors := poller.Start(ctx)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			if err := HandleSQSMessage(ctx, poller, message); err != nil {
				fmt.Fprintf(os.Stderr, "handle sqs message %s: %v\n", message.ExternalMessageID, err)
			}
		case err, ok := <-errors:
			if !ok {
				return
			}
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
