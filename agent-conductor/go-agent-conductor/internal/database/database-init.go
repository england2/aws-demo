package database

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed schema/database.sql
var db_init string

const databaseDir = "go-agent-conductor-runtime-database"

var db_path string

// debugForceNewDB reads the env flag that forces a fresh runtime database at startup.
// It is intentionally broad in accepted truthy values for shell convenience.
func debugForceNewDB() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_FORCE_NEW_DB")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// newestDatabasePath finds the newest conductor SQLite DB in the runtime directory.
// Startup uses this to resume recent state rather than always starting from scratch.
func newestDatabasePath(dir string) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, fmt.Errorf("read database dir: %w", err)
	}

	var newestPath string
	var newestModTime time.Time
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (name != "database.sqlite" && !(strings.HasPrefix(name, "database-") && strings.HasSuffix(name, ".sqlite"))) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return "", false, fmt.Errorf("stat database candidate %s: %w", name, err)
		}
		if newestPath == "" || info.ModTime().After(newestModTime) {
			newestPath = filepath.Join(dir, name)
			newestModTime = info.ModTime()
		}
	}

	return newestPath, newestPath != "", nil
}
