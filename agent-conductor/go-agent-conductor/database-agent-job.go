package main

import (
	"context"
	"database/sql"
	"fmt"

	dbgen "agent-orchestrator/internal/db/generated"
)

const defaultAgentJobName = "agent-fargate-codex"

func markAgentJobSpawned(ctx context.Context, db *sql.DB, agentJobID int64, taskARN string) DatabaseCommandResult {
	if taskARN == "" {
		return DatabaseCommandResult{Err: fmt.Errorf("task ARN is required to mark agent job spawned")}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin spawn success transaction: %w", err)}
	}
	defer tx.Rollback()

	row, err := dbgen.New(tx).MarkAgentJobSpawned(ctx, dbgen.MarkAgentJobSpawnedParams{
		Status:        string(AgentJobStatusRunning),
		EcsTaskArn:    sqlNullString(taskARN),
		EcsLastStatus: sqlNullString("PROVISIONING"),
		ID:            agentJobID,
	})
	agentJob, err := generatedAgentJob(row, err, "mark agent job spawned")
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusRunning); err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit spawn success transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: isTerminalAgentJobStatus(agentJob.Status),
		Reason:   "agent_job_spawned",
	}
}

func markAgentJobSpawnFailed(ctx context.Context, db *sql.DB, agentJobID int64, reason string) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin spawn failed transaction: %w", err)}
	}
	defer tx.Rollback()

	agentJob, err := failAgentJob(ctx, tx, agentJobID, defaultFailureReason(reason))
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit spawn failed transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: true,
		Reason:   "agent_job_spawn_failed",
	}
}

func updateAgentJobECSStatus(ctx context.Context, db *sql.DB, agentJobID int64, lastStatus string, stoppedReason string) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin ECS status transaction: %w", err)}
	}
	defer tx.Rollback()

	row, err := dbgen.New(tx).UpdateAgentJobECSStatus(ctx, dbgen.UpdateAgentJobECSStatusParams{
		EcsLastStatus:    sqlNullString(lastStatus),
		EcsStoppedReason: stoppedReason,
		ID:               agentJobID,
	})
	agentJob, err := generatedAgentJob(row, err, "update ECS status")
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit ECS status transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: isTerminalAgentJobStatus(agentJob.Status),
		Reason:   "agent_job_ecs_status_updated",
	}
}

func markAgentJobTaskStopped(ctx context.Context, db *sql.DB, agentJobID int64, lastStatus string, stoppedReason string, failureReason string) DatabaseCommandResult {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("begin task stopped transaction: %w", err)}
	}
	defer tx.Rollback()

	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}
	if isTerminalAgentJobStatus(current.Status) {
		if err := tx.Commit(); err != nil {
			return DatabaseCommandResult{Err: fmt.Errorf("commit already-terminal task stopped transaction: %w", err)}
		}
		return DatabaseCommandResult{
			AgentJob: &current,
			Terminal: true,
			Reason:   "agent_job_already_terminal",
		}
	}

	row, err := dbgen.New(tx).UpdateAgentJobECSStatus(ctx, dbgen.UpdateAgentJobECSStatusParams{
		EcsLastStatus:    sqlNullString(lastStatus),
		EcsStoppedReason: stoppedReason,
		ID:               agentJobID,
	})
	if _, err := generatedAgentJob(row, err, "record stopped ECS task"); err != nil {
		return DatabaseCommandResult{Err: err}
	}

	agentJob, err := failAgentJob(ctx, tx, agentJobID, defaultFailureReason(failureReason))
	if err != nil {
		return DatabaseCommandResult{Err: err}
	}

	if err := tx.Commit(); err != nil {
		return DatabaseCommandResult{Err: fmt.Errorf("commit task stopped transaction: %w", err)}
	}

	return DatabaseCommandResult{
		AgentJob: &agentJob,
		Terminal: true,
		Reason:   "agent_job_task_stopped_before_terminal_event",
	}
}

func createAgentJob(ctx context.Context, tx *sql.Tx, messageID int64) (DatabaseAgentJobInfo, error) {
	row, err := dbgen.New(tx).CreateAgentJob(ctx, dbgen.CreateAgentJobParams{
		AgentName:         defaultAgentJobName,
		Status:            string(AgentJobStatusCreated),
		SpawnSqsMessageID: messageID,
	})

	return generatedAgentJob(row, err, "create agent job")
}

func failAgentJob(ctx context.Context, tx *sql.Tx, agentJobID int64, reason string) (DatabaseAgentJobInfo, error) {
	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}
	if isTerminalAgentJobStatus(current.Status) {
		return current, nil
	}

	row, err := dbgen.New(tx).FailAgentJob(ctx, dbgen.FailAgentJobParams{
		Status:        string(AgentJobStatusFailed),
		FailureReason: sqlNullString(reason),
		ID:            agentJobID,
	})
	agentJob, err := generatedAgentJob(row, err, "mark agent job failed")
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusFailed); err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	return agentJob, nil
}

func succeedAgentJob(ctx context.Context, tx *sql.Tx, agentJobID int64, reportPath string) (DatabaseAgentJobInfo, error) {
	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}
	if isTerminalAgentJobStatus(current.Status) {
		return current, nil
	}

	row, err := dbgen.New(tx).SucceedAgentJob(ctx, dbgen.SucceedAgentJobParams{
		Status:      string(AgentJobStatusSucceeded),
		AgentReport: reportPath,
		ID:          agentJobID,
	})
	agentJob, err := generatedAgentJob(row, err, "mark agent job succeeded")
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusSucceeded); err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	return agentJob, nil
}

func markAgentJobRunning(ctx context.Context, tx *sql.Tx, agentJobID int64) (DatabaseAgentJobInfo, error) {
	current, err := selectAgentJobByID(ctx, tx, agentJobID)
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}
	if isTerminalAgentJobStatus(current.Status) {
		return current, nil
	}

	row, err := dbgen.New(tx).MarkAgentJobRunning(ctx, dbgen.MarkAgentJobRunningParams{
		Status: string(AgentJobStatusRunning),
		ID:     agentJobID,
	})
	agentJob, err := generatedAgentJob(row, err, "mark agent job running")
	if err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	if err := updateLinkedMessageStatusByAgentJobID(ctx, tx, agentJobID, AgentJobStatusRunning); err != nil {
		return DatabaseAgentJobInfo{}, err
	}

	return agentJob, nil
}

func recordPullRequest(ctx context.Context, tx *sql.Tx, agentJobID int64, pullRequestURL string) (DatabaseAgentJobInfo, error) {
	if pullRequestURL == "" {
		return selectAgentJobByID(ctx, tx, agentJobID)
	}

	row, err := dbgen.New(tx).RecordAgentJobPullRequest(ctx, dbgen.RecordAgentJobPullRequestParams{
		PullRequestUrl: sqlNullString(pullRequestURL),
		ID:             agentJobID,
	})

	return generatedAgentJob(row, err, "record pull request URL")
}

func selectAgentJobByID(ctx context.Context, tx *sql.Tx, agentJobID int64) (DatabaseAgentJobInfo, error) {
	row, err := dbgen.New(tx).GetAgentJobByID(ctx, agentJobID)

	return generatedAgentJob(row, err, "select agent job")
}

func generatedAgentJob(agentJob dbgen.AgentJobInfo, err error, operation string) (DatabaseAgentJobInfo, error) {
	if err != nil {
		return DatabaseAgentJobInfo{}, fmt.Errorf("%s: %w", operation, err)
	}

	return databaseAgentJobFromGenerated(agentJob), nil
}

func defaultFailureReason(reason string) string {
	if reason == "" {
		return "agent job failed"
	}

	return reason
}

func isTerminalAgentJobStatus(status AgentJobStatus) bool {
	return status == AgentJobStatusSucceeded || status == AgentJobStatusFailed
}
