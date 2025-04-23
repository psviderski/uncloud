package machine

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const DBFileName = "machine.db"

// NewDB creates a new connection to machine SQLite database and runs schema migrations if necessary.
func NewDB(path string) (*sqlx.DB, error) {
	// Create the database file with 0600 permissions if it doesn't exist, or update permissions if exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("create SQLite database '%s': %w", path, err)
		}
		file.Close()
	} else {
		if err = os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("update SQLite database permissions '%s': %w", path, err)
		}
	}

	// - Write-Ahead Logging (WAL) mode for better read/write performance.
	// - Busy timeout (5s) to make concurrent writes wait on each other instead of failing immediately.
	conn := path + "?_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL&_pragma=busy_timeout=5000&_time_format=sqlite"
	db, err := sqlx.Connect("sqlite", conn)
	if err != nil {
		return nil, fmt.Errorf("connect to SQLite database '%s': %w", conn, err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS containers (
	    id TEXT NOT NULL PRIMARY KEY,
	    service_spec TEXT NOT NULL CHECK (json_valid(service_spec)),
	    -- 'subsecond' modifier is used to store timestamps with millisecond precision.
	    created_at TIMESTAMP NOT NULL DEFAULT (datetime('subsecond')),
	    updated_at TIMESTAMP NOT NULL DEFAULT (datetime('subsecond'))
	);
    `

	if _, err = db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return db, nil
}
