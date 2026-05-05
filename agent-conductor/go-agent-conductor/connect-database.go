package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

type DatabaseConnectionInfo struct {
	DriverName     string
	DataSourceName string
	RuntimePath    string
}

type RuntimeDatabaseConnector interface {
	Reachable(context.Context) (bool, error)
	ConnectionInfo(context.Context) (DatabaseConnectionInfo, error)
	Open(context.Context) (*sql.DB, error)
}

type LocalSQLiteRuntimeDatabaseConnector struct {
	path string
}

func newLocalSQLiteRuntimeDatabaseConnector(path string) LocalSQLiteRuntimeDatabaseConnector {
	return LocalSQLiteRuntimeDatabaseConnector{path: path}
}

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

func initializeRuntimeDatabase(ctx context.Context) error {
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

func openConductorDB(ctx context.Context) (*sql.DB, error) {
	if db_path == "" {
		return nil, fmt.Errorf("db_path is empty; call initializeRuntimeDatabase first")
	}

	return newLocalSQLiteRuntimeDatabaseConnector(db_path).Open(ctx)
}
