package main

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// TestParseAgentEventSQSMessageRejectsNonNumericJobID protects the quarantine boundary.
// Non-numeric job IDs cannot map to agent_job_info rows and should not reach routers.
func TestParseAgentEventSQSMessageRejectsNonNumericJobID(t *testing.T) {
	message := sqstypes.Message{
		MessageId:     aws.String("test-message"),
		ReceiptHandle: aws.String("test-receipt"),
		Body: aws.String(`{
			"job_id": "manual-test",
			"agent_name": "agent-fargate-codex",
			"type": "CodexStarted"
		}`),
	}

	_, err := ParseAgentEventSQSMessage(message)
	if err == nil {
		t.Fatal("ParseAgentEventSQSMessage returned nil error, want invalid job_id error")
	}
	if !strings.Contains(err.Error(), `parse agent job id "manual-test"`) {
		t.Fatalf("error = %q, want invalid job_id error", err)
	}
}
