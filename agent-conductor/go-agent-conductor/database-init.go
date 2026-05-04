package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed database.sql
var db_init string

var db_path string

func check_load_db() error {
	if err := os.MkdirAll(databaseDir, 0755); err != nil {
		return fmt.Errorf("create database dir: %w", err)
	}

	init_db := func() error {
		newPath := filepath.Join(databaseDir, fmt.Sprintf("database-%d.sqlite", time.Now().UnixNano()))
		db, err := sql.Open("sqlite", newPath)
		if err != nil {
			return fmt.Errorf("open new database: %w", err)
		}
		defer db.Close()

		if _, err := db.Exec(db_init); err != nil {
			return fmt.Errorf("initialize new database %s: %w", newPath, err)
		}

		db_path = newPath
		return nil
	}

	if debugForceNewDB() {
		return init_db()
	}

	path, ok, err := newestDatabasePath(databaseDir)
	if err != nil {
		return err
	}
	if !ok {
		return init_db()
	}

	db_path = path

	if err := validateDatabase(db_path); err != nil {
		return init_db()
	}

	return nil
}

func debugForceNewDB() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_FORCE_NEW_DB")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func newestDatabasePath(dir string) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, fmt.Errorf("read database dir: %w", err)
	}

	var candidates []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name != "database.sqlite" && !(strings.HasPrefix(name, "database-") && strings.HasSuffix(name, ".sqlite")) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return "", false, fmt.Errorf("stat database candidate %s: %w", name, err)
		}

		candidates = append(candidates, info)
	}

	if len(candidates) == 0 {
		return "", false, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ModTime().After(candidates[j].ModTime())
	})

	return filepath.Join(dir, candidates[0].Name()), true, nil
}

func validateDatabase(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open database %s: %w", path, err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping database %s: %w", path, err)
	}

	requiredTables := []string{"sqs_messages_tickets_cloudwatch", "agent_job_info", "agent_event"}
	for _, table := range requiredTables {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
			table,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("check table %s: %w", table, err)
		}
		if count != 1 {
			return fmt.Errorf("missing table %s", table)
		}
	}

	if err := validateSQSMessageShape(db); err != nil {
		return err
	}
	if err := validateAgentJobShape(db); err != nil {
		return err
	}
	if err := validateAgentEventShape(db); err != nil {
		return err
	}

	return nil
}

func validateSQSMessageShape(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT
			id,
			external_message_id,
			receipt_handle,
			external_event_id,
			raw_body,
			message_type,
			cloudwatch_alarm_name,
			cloudwatch_state,
			event_time,
			alarm_period_seconds,
			assigned_agent_job_id,
			job_status,
			created_at,
			updated_at
		FROM sqs_messages_tickets_cloudwatch
		LIMIT 1
	`)
	if err != nil {
		return fmt.Errorf("validate sqs_messages_tickets_cloudwatch shape: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id                 int64
			externalMessageID  string
			receiptHandle      string
			externalEventID    sql.NullString
			rawBody            string
			messageType        string
			cloudwatchAlarm    sql.NullString
			cloudwatchState    sql.NullString
			eventTime          sql.NullString
			alarmPeriodSeconds sql.NullInt64
			assignedAgentJobID sql.NullInt64
			jobStatus          sql.NullString
			createdAt          string
			updatedAt          string
		)

		if err := rows.Scan(
			&id,
			&externalMessageID,
			&receiptHandle,
			&externalEventID,
			&rawBody,
			&messageType,
			&cloudwatchAlarm,
			&cloudwatchState,
			&eventTime,
			&alarmPeriodSeconds,
			&assignedAgentJobID,
			&jobStatus,
			&createdAt,
			&updatedAt,
		); err != nil {
			return fmt.Errorf("deserialize sqs_messages_tickets_cloudwatch sample: %w", err)
		}
	}

	return rows.Err()
}

func validateAgentJobShape(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT
			id,
			agent_name,
			status,
			spawn_sqs_message_id,
			agent_report,
			affected_repositories,
			pull_request_url,
			failure_reason,
			ecs_task_arn,
			ecs_last_status,
			ecs_stopped_reason,
			created_at,
			started_at,
			completed_at,
			updated_at
		FROM agent_job_info
		LIMIT 1
	`)
	if err != nil {
		return fmt.Errorf("validate agent_job_info shape: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id                int64
			agentName         string
			status            string
			spawnSQSMessageID int64
			agentReport       sql.NullString
			affectedRepos     sql.NullString
			pullRequestURL    sql.NullString
			failureReason     sql.NullString
			ecsTaskARN        sql.NullString
			ecsLastStatus     sql.NullString
			ecsStoppedReason  sql.NullString
			createdAt         string
			startedAt         sql.NullString
			completedAt       sql.NullString
			updatedAt         string
		)

		if err := rows.Scan(
			&id,
			&agentName,
			&status,
			&spawnSQSMessageID,
			&agentReport,
			&affectedRepos,
			&pullRequestURL,
			&failureReason,
			&ecsTaskARN,
			&ecsLastStatus,
			&ecsStoppedReason,
			&createdAt,
			&startedAt,
			&completedAt,
			&updatedAt,
		); err != nil {
			return fmt.Errorf("deserialize agent_job_info sample: %w", err)
		}
	}

	return rows.Err()
}

func validateAgentEventShape(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT
			id,
			event_id,
			agent_job_id,
			agent_name,
			event_type,
			message,
			report_path,
			artifact_url,
			raw_body,
			created_at,
			received_at
		FROM agent_event
		LIMIT 1
	`)
	if err != nil {
		return fmt.Errorf("validate agent_event shape: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id          int64
			eventID     string
			agentJobID  int64
			agentName   string
			eventType   string
			message     sql.NullString
			reportPath  sql.NullString
			artifactURL sql.NullString
			rawBody     string
			createdAt   sql.NullString
			receivedAt  string
		)

		if err := rows.Scan(
			&id,
			&eventID,
			&agentJobID,
			&agentName,
			&eventType,
			&message,
			&reportPath,
			&artifactURL,
			&rawBody,
			&createdAt,
			&receivedAt,
		); err != nil {
			return fmt.Errorf("deserialize agent_event sample: %w", err)
		}
	}

	return rows.Err()
}
