package main

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestCheckIsDbCompliantAcceptsExpectedSchema exercises the Atlas-backed startup guard.
// It starts from the embedded scheduler schema, creates a real SQLite database, and confirms the conductor would
// proceed to polling because Atlas reports no pending schema diff.
func TestCheckIsDbCompliantAcceptsExpectedSchema(t *testing.T) {
	skipIfAtlasCLIIsMissing(t)

	ctx := context.Background()
	schedulerDatabasePath := filepath.Join(t.TempDir(), "compliant.sqlite")
	createSQLiteDatabaseWithSchema(t, schedulerDatabasePath, expectedSchedulerDatabaseSchemaSQL)

	if !checkIsDbCompliant(ctx, schedulerDatabasePath) {
		t.Fatal("expected scheduler database to be compliant")
	}
}

// TestCheckIsDbCompliantRejectsMissingTables covers the noncompliant startup branch.
// It creates an empty SQLite database, then confirms main_testing would print the noncompliant message and return
// before polling SQS or opening the scheduler worker.
func TestCheckIsDbCompliantRejectsMissingTables(t *testing.T) {
	skipIfAtlasCLIIsMissing(t)

	ctx := context.Background()
	schedulerDatabasePath := filepath.Join(t.TempDir(), "noncompliant.sqlite")
	createSQLiteDatabaseWithSchema(t, schedulerDatabasePath, "")

	if checkIsDbCompliant(ctx, schedulerDatabasePath) {
		t.Fatal("expected empty scheduler database to be noncompliant")
	}
}

// TestDebugCreateNewDbAndSetLocationCreatesCompliantSibling covers the debug reset path.
// It starts from a requested database path, creates a timestamped sibling DB through Atlas, updates dbLocation, and
// confirms the created file satisfies the same compliance check main_testing uses before polling.
func TestDebugCreateNewDbAndSetLocationCreatesCompliantSibling(t *testing.T) {
	skipIfAtlasCLIIsMissing(t)

	originalDatabaseLocation := *dbLocation
	defer func() {
		*dbLocation = originalDatabaseLocation
	}()

	ctx := context.Background()
	requestedSchedulerDatabasePath := filepath.Join(t.TempDir(), "requested.sqlite")

	createdSchedulerDatabasePath, err := debugCreateNewDbAndSetLocation(ctx, requestedSchedulerDatabasePath)
	if err != nil {
		t.Fatalf("create debug database: %v", err)
	}

	if createdSchedulerDatabasePath == requestedSchedulerDatabasePath {
		t.Fatal("debug database should be a sibling path, not the requested path")
	}
	if *dbLocation != createdSchedulerDatabasePath {
		t.Fatalf("dbLocation = %q, want %q", *dbLocation, createdSchedulerDatabasePath)
	}
	if _, err := os.Stat(createdSchedulerDatabasePath); err != nil {
		t.Fatalf("stat created scheduler database: %v", err)
	}
	if !checkIsDbCompliant(ctx, createdSchedulerDatabasePath) {
		t.Fatal("created scheduler database should be compliant")
	}
}

func skipIfAtlasCLIIsMissing(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("atlas"); err != nil {
		t.Skip("atlas CLI is not installed")
	}
}

func createSQLiteDatabaseWithSchema(t *testing.T, schedulerDatabasePath string, schemaSQL string) {
	t.Helper()

	database, err := sql.Open("sqlite", schedulerDatabasePath)
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer database.Close()

	if schemaSQL == "" {
		if _, err := database.Exec("PRAGMA user_version = 0"); err != nil {
			t.Fatalf("create empty sqlite database: %v", err)
		}
		return
	}
	if _, err := database.Exec(schemaSQL); err != nil {
		t.Fatalf("create sqlite schema: %v", err)
	}
}
