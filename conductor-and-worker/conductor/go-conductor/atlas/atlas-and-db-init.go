package atlas

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed database.sql
var expectedSchemaSQL string

type databaseSchemaMismatchError struct {
	runtimeDatabasePath string
	diff                string
}

func (err databaseSchemaMismatchError) Error() string {
	return fmt.Sprintf("runtime database schema differs from db-sqlc/database.sql at %s:\n%s", err.runtimeDatabasePath, err.diff)
}

// IsDbCompliant compares the selected runtime SQLite database against the embedded scheduler schema.
// main calls it before opening the scheduler worker; false means startup should stop before polling SQS.
func IsDbCompliant(ctx context.Context, dbPath string) bool {
	if err := verifyRuntimeDatabaseSchema(ctx, dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "database compliance check failed: %v\n", err)
		return false
	}

	return true
}

// DbgCreateDb creates a fresh sibling DB that conforms to database.sql.
// It runs before scheduler.Open when debug_always_new_db is set, preserving the originally requested DB on disk.
func DbgCreateDb(ctx context.Context, requestedPath string) (string, error) {
	newDbPath, err := makeDebugDbPath(requestedPath)
	if err != nil {
		return "", err
	}
	if err := CreateConformantDatabase(ctx, newDbPath); err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "debug: created fresh scheduler database %s\n", newDbPath)
	return newDbPath, nil
}

// verifyRuntimeDatabaseSchema materializes the expected schema, then asks Atlas for a schema diff.
// It depends on the external atlas binary; Atlas' synced output means the runtime DB already matches database.sql.
func verifyRuntimeDatabaseSchema(ctx context.Context, runtimeDatabasePath string) error {
	if strings.TrimSpace(runtimeDatabasePath) == "" {
		return fmt.Errorf("scheduler database path is required")
	}
	if _, err := os.Stat(runtimeDatabasePath); err != nil {
		return fmt.Errorf("stat scheduler database %s: %w", runtimeDatabasePath, err)
	}

	desiredDatabasePath, cleanup, err := createDesiredDatabaseSnapshot(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	diff, err := atlasSchemaDiff(ctx, runtimeDatabasePath, desiredDatabasePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(diff) != "" {
		return databaseSchemaMismatchError{
			runtimeDatabasePath: runtimeDatabasePath,
			diff:                strings.TrimSpace(diff),
		}
	}

	return nil
}

// createDesiredDatabaseSnapshot writes the embedded schema into a temporary SQLite file.
// Atlas compares the selected runtime DB against this real SQLite database instead of comparing raw SQL text.
func createDesiredDatabaseSnapshot(ctx context.Context) (string, func(), error) {
	temporaryDirectory, err := os.MkdirTemp("", "go-conductor-desired-db-*")
	if err != nil {
		return "", nil, fmt.Errorf("create desired database temp dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(temporaryDirectory)
	}

	desiredDatabasePath := filepath.Join(temporaryDirectory, "desired.sqlite")
	database, err := sql.Open("sqlite", desiredDatabasePath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open desired database snapshot: %w", err)
	}
	defer database.Close()

	if _, err := database.ExecContext(ctx, expectedSchemaSQL); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("initialize desired database snapshot: %w", err)
	}

	return desiredDatabasePath, cleanup, nil
}

// CreateConformantDatabase creates an empty SQLite file with the embedded scheduler schema.
// It is shared by main's debug DB path and scheduler tests, and it refuses to overwrite an existing path.
func CreateConformantDatabase(ctx context.Context, schedulerDatabasePath string) error {
	if err := os.MkdirAll(filepath.Dir(schedulerDatabasePath), 0o755); err != nil {
		return fmt.Errorf("create debug database dir: %w", err)
	}

	databaseFile, err := os.OpenFile(schedulerDatabasePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create debug scheduler database %s: %w", schedulerDatabasePath, err)
	}
	if err := databaseFile.Close(); err != nil {
		return fmt.Errorf("close debug scheduler database %s: %w", schedulerDatabasePath, err)
	}

	database, err := sql.Open("sqlite", schedulerDatabasePath)
	if err != nil {
		_ = os.Remove(schedulerDatabasePath)
		return fmt.Errorf("open debug scheduler database %s: %w", schedulerDatabasePath, err)
	}
	defer database.Close()

	if _, err := database.ExecContext(ctx, expectedSchemaSQL); err != nil {
		_ = os.Remove(schedulerDatabasePath)
		return fmt.Errorf("initialize debug scheduler database %s: %w", schedulerDatabasePath, err)
	}

	return nil
}

// atlasSchemaDiff returns Atlas' migration plan from the runtime DB to the desired DB.
// The installed Atlas CLI prints a synced sentence instead of structured JSON when no changes are needed.
func atlasSchemaDiff(ctx context.Context, fromDatabasePath string, toDatabasePath string) (string, error) {
	fromDatabaseURL, err := sqliteDatabaseURL(fromDatabasePath)
	if err != nil {
		return "", err
	}
	toDatabaseURL, err := sqliteDatabaseURL(toDatabasePath)
	if err != nil {
		return "", err
	}

	output, err := runAtlasCommand(ctx, "schema", "diff", "--from", fromDatabaseURL, "--to", toDatabaseURL)
	return normalizeAtlasDiffOutput(output), err
}

// atlasSchemaApply applies the desired SQLite schema to the target SQLite database.
// debugCreateNewDbAndSetLocation uses it only for newly-created empty debug databases.
func atlasSchemaApply(ctx context.Context, targetDatabasePath string, desiredDatabasePath string) error {
	targetDatabaseURL, err := sqliteDatabaseURL(targetDatabasePath)
	if err != nil {
		return err
	}

	desiredSchemaHCL, err := atlasSchemaInspectHCL(ctx, desiredDatabasePath)
	if err != nil {
		return err
	}

	temporaryDirectory, err := os.MkdirTemp("", "go-conductor-atlas-schema-*")
	if err != nil {
		return fmt.Errorf("create atlas schema temp dir: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	desiredSchemaPath := filepath.Join(temporaryDirectory, "schema.hcl")
	if err := os.WriteFile(desiredSchemaPath, []byte(desiredSchemaHCL), 0o600); err != nil {
		return fmt.Errorf("write atlas schema file: %w", err)
	}

	_, err = runAtlasCommand(ctx, "schema", "apply", "-d", targetDatabaseURL, "-f", desiredSchemaPath, "--auto-approve")
	return err
}

// atlasSchemaInspectHCL asks the installed Atlas CLI to render a SQLite database as Atlas HCL.
// debug DB creation uses this because the local CLI accepts -f schema.hcl but not the newer --to database option.
func atlasSchemaInspectHCL(ctx context.Context, databasePath string) (string, error) {
	databaseURL, err := sqliteDatabaseURL(databasePath)
	if err != nil {
		return "", err
	}

	return runAtlasCommand(ctx, "schema", "inspect", "-d", databaseURL)
}

// runAtlasCommand invokes the locally installed Atlas CLI and returns combined output.
// Using the CLI directly matches the installed development Atlas binary, whose flags predate atlasexec's JSON mode.
func runAtlasCommand(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "atlas", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("run atlas %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}

// normalizeAtlasDiffOutput collapses Atlas' no-op diff message into an empty string.
// Any remaining output is a real migration plan and therefore marks the runtime DB as noncompliant.
func normalizeAtlasDiffOutput(output string) string {
	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" || strings.Contains(trimmedOutput, "Schemas are synced, no changes to be made.") {
		return ""
	}

	return trimmedOutput
}

// makeDebugDbPath chooses a timestamped SQLite path next to the requested runtime DB.
// The original path is not modified; the caller replaces dbLocation only after Atlas creates the new file.
func makeDebugDbPath(requestedPath string) (string, error) {
	RequestedAbsPath, err := filepath.Abs(requestedPath)
	if err != nil {
		return "", fmt.Errorf("resolve scheduler database path %s: %w", requestedPath, err)
	}

	dbDir := filepath.Dir(RequestedAbsPath)
	dbExtension := filepath.Ext(RequestedAbsPath)
	if dbExtension == "" {
		dbExtension = ".sqlite"
	}
	dbName := strings.TrimSuffix(filepath.Base(RequestedAbsPath), filepath.Ext(RequestedAbsPath))
	if dbName == "" || dbName == "." {
		dbName = "scheduler"
	}

	timestamp := time.Now().UTC().Format("20060102-150405.000000000")
	return filepath.Join(dbDir, fmt.Sprintf("%s-debug-%s%s", dbName, timestamp, dbExtension)), nil
}

// sqliteDatabaseURL converts a filesystem path into an Atlas-compatible SQLite URL.
// filepath.ToSlash keeps the URL stable across platforms even though deployment is currently Linux.
func sqliteDatabaseURL(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve sqlite path %s: %w", path, err)
	}

	return "sqlite://file:" + filepath.ToSlash(absolutePath), nil
}
