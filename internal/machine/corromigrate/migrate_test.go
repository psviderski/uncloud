package corromigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestIsV0Store(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*testing.T, *sql.DB)
		expectV0 bool
	}{
		{
			name: "v0.x store has __corro_bookkeeping table",
			setup: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec("CREATE TABLE __corro_bookkeeping (actor_id BLOB, version INTEGER)")
				require.NoError(t, err)
			},
			expectV0: true,
		},
		{
			name: "v1.0.0 store has only __corro_bookkeeping_gaps",
			setup: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec("CREATE TABLE __corro_bookkeeping_gaps (actor_id BLOB, start INTEGER, end INTEGER)")
				require.NoError(t, err)
			},
			expectV0: false,
		},
		{
			name: "empty store has neither table",
			setup: func(t *testing.T, db *sql.DB) {
				// Force the SQLite file to materialize on disk so it can be reopened read-only.
				_, err := db.Exec("PRAGMA user_version = 0")
				require.NoError(t, err)
			},
			expectV0: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "store.db")
			db, err := sql.Open("sqlite", dbPath)
			require.NoError(t, err)
			tt.setup(t, db)
			require.NoError(t, db.Close())

			got, err := isV0Store(context.Background(), dbPath)
			require.NoError(t, err)
			assert.Equal(t, tt.expectV0, got)
		})
	}
}

func TestDumpOldStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE cluster (key TEXT PRIMARY KEY, value ANY)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE machines (id TEXT PRIMARY KEY, info TEXT)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO cluster (key, value) VALUES (?, ?)`,
		"network", "10.210.0.0/16")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO cluster (key, value) VALUES (?, ?)`,
		"created_at", "2026-05-22T10:00:00Z")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO machines (id, info) VALUES (?, ?)`,
		"m1", `{"id":"m1","name":"alpha"}`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO machines (id, info) VALUES (?, ?)`,
		"m2", `{"id":"m2","name":"beta"}`)
	require.NoError(t, err)

	require.NoError(t, db.Close())

	seed, err := dumpOldStore(context.Background(), dbPath)
	require.NoError(t, err)

	assert.ElementsMatch(t, []ClusterEntry{
		{Key: "network", Value: "10.210.0.0/16"},
		{Key: "created_at", Value: "2026-05-22T10:00:00Z"},
	}, seed.Cluster)

	assert.ElementsMatch(t, []MachineEntry{
		{ID: "m1", Info: `{"id":"m1","name":"alpha"}`},
		{ID: "m2", Info: `{"id":"m2","name":"beta"}`},
	}, seed.Machines)
}

func TestMigrateIfNeeded(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		wantSeed   bool
		wantBackup bool
	}{
		{
			name:       "greenfield (no store.db) is a no-op",
			setup:      func(t *testing.T, dir string) { require.NoError(t, os.MkdirAll(dir, 0o700)) },
			wantSeed:   false,
			wantBackup: false,
		},
		{
			name: "v1.0.0 store (no __corro_bookkeeping) is a no-op",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(dir, 0o700))
				dbPath := filepath.Join(dir, "store.db")
				db, err := sql.Open("sqlite", dbPath)
				require.NoError(t, err)
				_, err = db.Exec(`CREATE TABLE __corro_bookkeeping_gaps (actor_id BLOB)`)
				require.NoError(t, err)
				require.NoError(t, db.Close())
			},
			wantSeed:   false,
			wantBackup: false,
		},
		{
			name: "v0.x store triggers dump, seed, and backup",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(dir, 0o700))
				dbPath := filepath.Join(dir, "store.db")
				db, err := sql.Open("sqlite", dbPath)
				require.NoError(t, err)
				_, err = db.Exec(`CREATE TABLE __corro_bookkeeping (actor_id BLOB, version INTEGER)`)
				require.NoError(t, err)
				_, err = db.Exec(`CREATE TABLE cluster (key TEXT PRIMARY KEY, value ANY)`)
				require.NoError(t, err)
				_, err = db.Exec(`CREATE TABLE machines (id TEXT PRIMARY KEY, info TEXT)`)
				require.NoError(t, err)
				_, err = db.Exec(`INSERT INTO cluster VALUES (?, ?)`, "network", "10.210.0.0/16")
				require.NoError(t, err)
				_, err = db.Exec(`INSERT INTO machines VALUES (?, ?)`, "m1", `{"id":"m1"}`)
				require.NoError(t, err)
				require.NoError(t, db.Close())
			},
			wantSeed:   true,
			wantBackup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "corrosion")
			tt.setup(t, dir)

			err := MigrateIfNeeded(context.Background(), dir, "")
			require.NoError(t, err)

			seedPath := dir + ".seed-v1.json"
			_, seedErr := os.Stat(seedPath)
			if tt.wantSeed {
				require.NoError(t, seedErr, "seed file expected")

				data, err := os.ReadFile(seedPath)
				require.NoError(t, err)
				var seed Seed
				require.NoError(t, json.Unmarshal(data, &seed))
				assert.Len(t, seed.Cluster, 1)
				assert.Len(t, seed.Machines, 1)
			} else {
				assert.True(t, os.IsNotExist(seedErr), "seed file unexpected")
			}

			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			var backups int
			for _, e := range entries {
				if e.Name() != "corrosion" && len(e.Name()) > len("corrosion.backup-") &&
					e.Name()[:len("corrosion.backup-")] == "corrosion.backup-" {
					backups++
				}
			}
			if tt.wantBackup {
				assert.Equal(t, 1, backups, "expected one backup dir")
				_, err := os.Stat(filepath.Join(dir, "store.db"))
				assert.True(t, os.IsNotExist(err), "store.db should be gone from recreated dir")
			} else {
				assert.Equal(t, 0, backups, "no backup expected")
			}
		})
	}
}

func TestMigrateIfNeeded_SkipsDumpWhenSeedAlreadyExists(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "corrosion")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	dbPath := filepath.Join(dir, "store.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE __corro_bookkeeping (x INTEGER)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE cluster (key TEXT, value ANY)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE machines (id TEXT, info TEXT)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	preseed := Seed{Cluster: []ClusterEntry{{Key: "preseed", Value: "v"}}}
	data, err := json.Marshal(&preseed)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dir+".seed-v1.json", data, 0o600))

	require.NoError(t, MigrateIfNeeded(context.Background(), dir, ""))

	// Existing seed must be left untouched (no dump performed, no backup created).
	got, err := os.ReadFile(dir + ".seed-v1.json")
	require.NoError(t, err)
	var afterSeed Seed
	require.NoError(t, json.Unmarshal(got, &afterSeed))
	assert.Equal(t, preseed, afterSeed)

	// The corrosion dir should still be present (not backed up).
	_, err = os.Stat(filepath.Join(dir, "store.db"))
	assert.NoError(t, err)
}
