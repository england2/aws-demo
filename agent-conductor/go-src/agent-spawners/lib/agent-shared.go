package lib

import (
	"context"
	"time"
)

// agent-shared.go: interface and shared structs for agent-spawner definitions.

type SQSMessage struct {
	MessageID     string
	ReceiptHandle string
	Body          []byte
	RawBody       string
}

// PointReason will be "read only", and every time we run  be defined inline in the DecideRunSQSMessage message
type JobPoint struct {
	Points      int
	PointReason string
}

type AgentMatch struct {
	AgentSpawnerName string
	// JobPoints is the score explanation an agent-spawner uses to compete with
	// other agent-spawners when multiple want to handle a job
	JobPoints []JobPoint
}

func (m *AgentMatch) AddJobPoint(points int, reason string) {
	m.JobPoints = append(m.JobPoints, JobPoint{
		Points:      points,
		PointReason: reason,
	})
}

func (m AgentMatch) TotalPoints() int {
	total := 0
	for _, point := range m.JobPoints {
		total += point.Points
	}

	return total
}

type SpawnedAgent struct {
	JobID     int64
	AgentName string
	RuntimeID string
	// EventStream and AgentEvents will be read from by a consumer at one point
	EventStream <-chan AgentEvent
	Cancel      context.CancelFunc
	StartedAt   time.Time
}

type AgentSpawnerSpec struct {
	Name        string
	Description string
}

type AgentSpawner interface {
	Spec() AgentSpawnerSpec
	// DecideRunSQSMessage consumes a message from the shared SQS queue and returns
	// a scored match when this spawner is a candidate to handle it.
	DecideRunSQSMessage(message SQSMessage) (AgentMatch, error)
}
