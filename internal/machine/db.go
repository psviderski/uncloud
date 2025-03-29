package machine

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const DBFileName = "machine.db"

// NewDB creates a new connection to machine SQLite database and runs schema migrations if necessary.
func NewDB(path string) (*sqlx.DB, error) {
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
	    service_id TEXT NOT NULL,
	    service_name TEXT AS (json_extract(service_spec, '$.Name')),
	    service_spec TEXT NOT NULL CHECK (json_valid(service_spec)),
	    -- 'subsecond' modifier is used to store timestamps with millisecond precision.
	    created_at TIMESTAMP NOT NULL DEFAULT (datetime('subsecond')),
	    updated_at TIMESTAMP NOT NULL DEFAULT (datetime('subsecond'))
	);

	CREATE INDEX IF NOT EXISTS idx_containers_service_id ON containers (service_id);
	CREATE INDEX IF NOT EXISTS idx_containers_service_name ON containers (service_name);
    `

	if _, err = db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return db, nil
}
