package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func createRuntimeDatabase(ctx context.Context) (string, error) {
	return createRuntimeDatabaseFile(ctx, false)
}

func debugCreateRuntimeDatabase(ctx context.Context) (string, error) {
	return createRuntimeDatabaseFile(ctx, true)
}

func debugRecreateMismatchedDB() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_RECREATE_MISMATCHED_DB")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

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
