package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// DatabaseConnectionInfo describes how the conductor should open the runtime DB.
// It exists so the connector abstraction can later support a non-local backing
// store without changing callers that only need driver, DSN, and runtime path.
type DatabaseConnectionInfo struct {
	DriverName     string
	DataSourceName string
	RuntimePath    string
}

// RuntimeDatabaseConnector is the conductor's small database access boundary.
// Startup code uses it to check reachability, retrieve connection metadata, and
// open a configured DB. The current implementation is local SQLite only.
type RuntimeDatabaseConnector interface {
	Reachable(context.Context) (bool, error)
	ConnectionInfo(context.Context) (DatabaseConnectionInfo, error)
	Open(context.Context) (*sql.DB, error)
}

// LocalSQLiteRuntimeDatabaseConnector connects the conductor to one SQLite file.
// It depends on modernc.org/sqlite being registered and on databaseDir/db_path
// selection happening before the database worker starts.
type LocalSQLiteRuntimeDatabaseConnector struct {
	path string
}

// newLocalSQLiteRuntimeDatabaseConnector wraps a chosen SQLite path as a connector.
// The path is normally selected by initializeRuntimeDatabase during conductor startup.
func newLocalSQLiteRuntimeDatabaseConnector(path string) LocalSQLiteRuntimeDatabaseConnector {
	return LocalSQLiteRuntimeDatabaseConnector{path: path}
}

// Reachable checks whether the configured SQLite file exists and is not a directory.
// This is intentionally lightweight; schema compatibility is verified separately by Atlas.
func (c LocalSQLiteRuntimeDatabaseConnector) Reachable(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	info, err := os.Stat(c.path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat database %s: %w", c.path, err)
}

// ConnectionInfo returns sql.Open metadata after confirming the SQLite file is usable.
// Callers use RuntimePath for schema diffing and diagnostic messages.
func (c LocalSQLiteRuntimeDatabaseConnector) ConnectionInfo(ctx context.Context) (DatabaseConnectionInfo, error) {
	reachable, err := c.Reachable(ctx)
	if err != nil {
		return DatabaseConnectionInfo{}, err
	}
	if !reachable {
		return DatabaseConnectionInfo{}, fmt.Errorf("database is not reachable at %s", c.path)
	}

	return DatabaseConnectionInfo{
		DriverName:     "sqlite",
		DataSourceName: c.path,
		RuntimePath:    c.path,
	}, nil
}

// Open creates and validates a live SQLite connection for conductor DB operations.
// It configures foreign key enforcement and a busy timeout so the single DB worker
// and any future readers behave predictably against the local file.
func (c LocalSQLiteRuntimeDatabaseConnector) Open(ctx context.Context) (*sql.DB, error) {
	info, err := c.ConnectionInfo(ctx)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(info.DriverName, info.DataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", info.RuntimePath, err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database %s: %w", info.RuntimePath, err)
	}

	if _, err := db.ExecContext(ctx, `
		PRAGMA foreign_keys = ON;
		PRAGMA busy_timeout = 5000;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database pragmas: %w", err)
	}

	return db, nil
}

// InitializeRuntimeDatabase chooses or creates the SQLite database for this process.
// It is the startup gate before the DB worker runs: existing DBs must match the
// embedded schema, unless debug env opts into creating a fresh replacement DB.
func InitializeRuntimeDatabase(ctx context.Context) error {
	if err := os.MkdirAll(databaseDir, 0755); err != nil {
		return fmt.Errorf("create database dir: %w", err)
	}

	if debugForceNewDB() {
		path, err := debugCreateRuntimeDatabase(ctx)
		if err != nil {
			return err
		}
		db_path = path
		return nil
	}

	path, ok, err := newestDatabasePath(databaseDir)
	if err != nil {
		return err
	}
	if !ok {
		path, err := createRuntimeDatabase(ctx)
		if err != nil {
			return err
		}
		db_path = path
		return nil
	}

	connector := newLocalSQLiteRuntimeDatabaseConnector(path)
	info, err := connector.ConnectionInfo(ctx)
	if err != nil {
		return err
	}

	if err := verifyRuntimeDatabaseSchema(ctx, info.RuntimePath); err != nil {
		var mismatch DatabaseSchemaMismatchError
		if debugRecreateMismatchedDB() && errors.As(err, &mismatch) {
			path, err := debugCreateRuntimeDatabase(ctx)
			if err != nil {
				return err
			}
			db_path = path
			return nil
		}
		return err
	}

	db_path = info.RuntimePath
	return nil
}

// openConductorDB opens the globally selected runtime database for the DB worker.
// It depends on InitializeRuntimeDatabase having already set db_path.
func openConductorDB(ctx context.Context) (*sql.DB, error) {
	if db_path == "" {
		return nil, fmt.Errorf("db_path is empty; call initializeRuntimeDatabase first")
	}

	return newLocalSQLiteRuntimeDatabaseConnector(db_path).Open(ctx)
}
