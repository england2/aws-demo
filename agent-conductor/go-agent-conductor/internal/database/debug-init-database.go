package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// createRuntimeDatabase creates a fresh non-debug SQLite runtime database.
// Startup uses this when no previous runtime database exists.
func createRuntimeDatabase(ctx context.Context) (string, error) {
	return createRuntimeDatabaseFile(ctx, false)
}

// debugCreateRuntimeDatabase creates a fresh runtime DB and logs the chosen path.
// It is used by debug startup paths that intentionally avoid reusing an old DB.
func debugCreateRuntimeDatabase(ctx context.Context) (string, error) {
	return createRuntimeDatabaseFile(ctx, true)
}

// debugRecreateMismatchedDB reads the opt-in env flag for replacing drifted DBs.
// This keeps normal startup strict while allowing fast local/EC2 debug resets.
func debugRecreateMismatchedDB() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_RECREATE_MISMATCHED_DB")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// createRuntimeDatabaseFile writes database.sql into a new timestamped SQLite file.
// The timestamped name avoids overwriting previous debug databases while preserving
// enough state on disk for manual inspection after a failed conductor run.
func createRuntimeDatabaseFile(ctx context.Context, debug bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.MkdirAll(databaseDir, 0755); err != nil {
		return "", fmt.Errorf("create database dir: %w", err)
	}

	newPath := filepath.Join(databaseDir, fmt.Sprintf("database-%d.sqlite", time.Now().UnixNano()))
	db, err := sql.Open("sqlite", newPath)
	if err != nil {
		return "", fmt.Errorf("open new database: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, db_init); err != nil {
		return "", fmt.Errorf("initialize new database %s: %w", newPath, err)
	}

	if debug {
		fmt.Fprintf(os.Stderr, "debug: created fresh runtime database %s\n", newPath)
	}

	return newPath, nil
}
