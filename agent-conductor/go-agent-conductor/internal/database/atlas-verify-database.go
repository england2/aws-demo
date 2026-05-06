package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"ariga.io/atlas/atlasexec"
)

// DatabaseSchemaMismatchError reports a runtime SQLite schema drift from database.sql.
// initializeRuntimeDatabase can either fail on this error or create a debug fresh DB.
type DatabaseSchemaMismatchError struct {
	RuntimePath string
	Diff        string
}

// Error formats the Atlas diff so startup failures include actionable schema details.
// The caller prints this directly when the conductor refuses to use a mismatched DB.
func (e DatabaseSchemaMismatchError) Error() string {
	return fmt.Sprintf("runtime database schema differs from embedded database.sql at %s:\n%s", e.RuntimePath, e.Diff)
}

// verifyRuntimeDatabaseSchema compares the selected runtime DB against database.sql.
// It creates a temporary desired SQLite DB, then asks Atlas for a dry-run schema diff.
func verifyRuntimeDatabaseSchema(ctx context.Context, runtimePath string) error {
	desiredPath, cleanup, err := createDesiredDatabaseSnapshot()
	if err != nil {
		return err
	}
	defer cleanup()

	diff, err := atlasSchemaDiff(ctx, runtimePath, desiredPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(diff) != "" {
		return DatabaseSchemaMismatchError{
			RuntimePath: runtimePath,
			Diff:        strings.TrimSpace(diff),
		}
	}

	return nil
}

// createDesiredDatabaseSnapshot materializes the embedded schema into a temp DB file.
// Atlas compares this file against the runtime DB because both sides are real SQLite URLs.
func createDesiredDatabaseSnapshot() (string, func(), error) {
	dir, err := os.MkdirTemp("", "go-agent-conductor-desired-db-*")
	if err != nil {
		return "", nil, fmt.Errorf("create desired database temp dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	path := filepath.Join(dir, "desired.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open desired database snapshot: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(db_init); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("initialize desired database snapshot: %w", err)
	}

	return path, cleanup, nil
}

// atlasSchemaDiff returns pending Atlas changes from runtime DB to desired DB.
// It depends on the external `atlas` binary being installed on the conductor host.
// Empty output means the runtime schema already matches the embedded schema.
func atlasSchemaDiff(ctx context.Context, fromPath string, toPath string) (string, error) {
	fromURL, err := sqliteDatabaseURL(fromPath)
	if err != nil {
		return "", err
	}
	toURL, err := sqliteDatabaseURL(toPath)
	if err != nil {
		return "", err
	}

	client, err := atlasexec.NewClient("", "atlas")
	if err != nil {
		return "", fmt.Errorf("create atlas client: %w", err)
	}

	result, err := client.SchemaApply(ctx, &atlasexec.SchemaApplyParams{
		URL:    fromURL,
		To:     toURL,
		DevURL: "sqlite://dev?mode=memory",
		DryRun: true,
	})
	if err != nil {
		return "", fmt.Errorf("run atlas schema apply dry-run: %w", err)
	}
	if result == nil {
		return "", nil
	}

	return strings.Join(result.Changes.Pending, "\n"), nil
}

// sqliteDatabaseURL converts a filesystem path into an Atlas-compatible SQLite URL.
// filepath.ToSlash keeps the URL stable across platforms even though deployment is Debian.
func sqliteDatabaseURL(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve sqlite path %s: %w", path, err)
	}

	return (&url.URL{
		Scheme: "sqlite",
		Path:   filepath.ToSlash(absPath),
	}).String(), nil
}
