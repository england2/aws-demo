package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	scheduler "go-conductor/go-db-scheduler"
)

type sqsMessageDeleter interface {
	DeleteMessage(ctx context.Context, receiptHandle string) error
}

// runSchedulerAndPrintDecisions is the one-shot scheduler probe used by local test entry points.
// It runs after the DB path flag has been parsed and prints the decisions returned by the scheduler package.
//
// ai--done
func runSchedulerAndPrintDecisions(ctx context.Context, schedulerDatabasePath string) error {
	decisions, err := scheduler.Run(ctx, scheduler.Config{
		DBPath: schedulerDatabasePath,
	})
	if err != nil {
		return err
	}

	return printSchedulerDecisions(decisions)
}

// printSchedulerDecisions renders scheduler output for the ad hoc main_testing flow.
// It is called after RunScheduling returns so polling logs show the exact spawn decisions produced for a message.
func printSchedulerDecisions(scheduleDecisions any) error {
	encoded, err := json.MarshalIndent(scheduleDecisions, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(encoded))
	return nil
}

// schedulerIncomingMessageFromPolledSQSMessage adapts the poller's transport shape into the scheduler's DB input.
// It runs after ParseSQSMessageBody has extracted EventBridge metadata and preserves RawBody as the scheduler's
// authoritative message text while supplying the account number required by scheduler inserts.
//
// ai--done
func schedulerIncomingMessageFromPolledSQSMessage(polledSQSMessage PolledSQSMessage, parsedSQSMessage ParsedSQSMessage) (scheduler.IncomingMessage, error) {
	if parsedSQSMessage.AccountNumber == nil || strings.TrimSpace(*parsedSQSMessage.AccountNumber) == "" {
		return scheduler.IncomingMessage{}, fmt.Errorf("sqs message %q is missing aws account number", polledSQSMessage.ExternalMessageID)
	}

	return scheduler.IncomingMessage{
		RawBody:       polledSQSMessage.Body,
		AccountNumber: strings.TrimSpace(*parsedSQSMessage.AccountNumber),
	}, nil
}

// insertPolledSQSMessageAndRunScheduler persists one supported SQS delivery and asks the scheduler for decisions.
// It is called by main_testing after the poller emits a message; DeleteMessage happens only after the insert and
// scheduling pass both complete, so failed messages remain visible for later retry. Because scheduler rows do not
// store an external message ID yet, a successful insert followed by a failed SQS delete can be redelivered as a duplicate.
//
// ai--done
func insertPolledSQSMessageAndRunScheduler(ctx context.Context, schedulerWorker *scheduler.Worker, sqsDeleter sqsMessageDeleter, polledSQSMessage PolledSQSMessage) error {
	parsedSQSMessage := ParseSQSMessageBody([]byte(polledSQSMessage.Body))
	switch parsedSQSMessage.MessageType {
	case MessageTypeCloudWatchAlarm:
		schedulerIncomingMessage, err := schedulerIncomingMessageFromPolledSQSMessage(polledSQSMessage, parsedSQSMessage)
		if err != nil {
			return err
		}
		if err := schedulerWorker.InsertAlarmMessages(ctx, []scheduler.IncomingMessage{schedulerIncomingMessage}); err != nil {
			return fmt.Errorf("insert alarm message: %w", err)
		}
	default:
		return fmt.Errorf("unsupported sqs message type %q for message %q", parsedSQSMessage.MessageType, polledSQSMessage.ExternalMessageID)
	}

	scheduleDecisions, err := schedulerWorker.RunScheduling(ctx)
	if err != nil {
		return fmt.Errorf("run scheduler: %w", err)
	}
	if err := printSchedulerDecisions(scheduleDecisions); err != nil {
		return fmt.Errorf("print scheduler decisions: %w", err)
	}

	if err := sqsDeleter.DeleteMessage(ctx, polledSQSMessage.ReceiptHandle); err != nil {
		return err
	}

	return nil
}

// testSchedulerDatabasePathFromFlags resolves the database path for ad hoc conductor test entry points.
// It runs after flag.Parse and accepts either -test-db-loc or a positional path after "--", keeping the polling
// smoke test compatible with normal Go flag parsing while still feeding scheduler.Open a single DB path string.
//
// ai--done
func testSchedulerDatabasePathFromFlags() string {
	if strings.TrimSpace(*dbLocation) != "" {
		return strings.TrimSpace(*dbLocation)
	}
	if flag.NArg() > 0 {
		return strings.TrimSpace(flag.Arg(0))
	}

	return ""
}
