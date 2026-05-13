package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"go-conductor/db-internal/shared"
	scheduler "go-conductor/go-db-scheduler"
)

// runSchedulerAndPrintDecisions is the one-shot scheduler probe used by local test entry points.
// It runs after the DB path flag has been parsed and prints the decisions returned by the scheduler package.
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
// ai--done
func printSchedulerDecisions(scheduleDecisions []shared.ScheduleDecision) error {
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
// It is called by main_testing after the poller emits a message, and it returns schedule decisions without touching
// SQS so the top-level polling loop remains responsible for external queue delete ordering.
func insertPolledSQSMessageAndRunScheduler(ctx context.Context, schedulerWorker *scheduler.Worker, polledSQSMessage PolledSQSMessage) ([]shared.ScheduleDecision, error) {
	parsedSQSMessage := ParseSQSMessageBody([]byte(polledSQSMessage.Body))
	switch parsedSQSMessage.MessageType {
	case MessageTypeCloudWatchAlarm:
		schedulerIncomingMessage, err := schedulerIncomingMessageFromPolledSQSMessage(polledSQSMessage, parsedSQSMessage)
		if err != nil {
			return nil, err
		}
		return schedulerWorker.InsertAlarmMessageAndRunScheduling(ctx, schedulerIncomingMessage)
	default:
		return nil, fmt.Errorf("unsupported sqs message type %q for message %q", parsedSQSMessage.MessageType, polledSQSMessage.ExternalMessageID)
	}
}

// testSchedulerDatabasePathFromFlags resolves the database path for ad hoc conductor test entry points.
// It runs after flag.Parse and accepts either -test-db-loc or a positional path after "--", keeping the polling
// smoke test compatible with normal Go flag parsing while still feeding scheduler.Open a single DB path string.
func testSchedulerDatabasePathFromFlags() string {
	return schedulerDatabasePathFromFlagValues(*dbLocation, flag.Args())
}

// schedulerDatabasePathFromFlagValues accepts both normal -test-db-loc parsing and the ad hoc "-- test-db-loc path" form.
// The latter appears as positional args after flag.Parse, so main_testing normalizes it before opening or verifying SQLite.
func schedulerDatabasePathFromFlagValues(configuredDatabaseLocation string, flagArguments []string) string {
	if strings.TrimSpace(configuredDatabaseLocation) != "" {
		return strings.TrimSpace(configuredDatabaseLocation)
	}

	if len(flagArguments) == 0 {
		return ""
	}
	if len(flagArguments) >= 2 {
		flagLikeName := strings.TrimLeft(flagArguments[0], "-")
		if flagLikeName == "test-db-loc" || flagLikeName == "database-loc" {
			return strings.TrimSpace(flagArguments[1])
		}
	}

	return strings.TrimSpace(flagArguments[0])
}
