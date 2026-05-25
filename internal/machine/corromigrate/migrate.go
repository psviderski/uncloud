// Package corromigrate handles the one-time migration of the on-disk Corrosion store from
// systemd-managed v0.x to v1.0.0 running in a uncloudd-managed container.
// TODO: remove in 0.22 assuming all pre 0.20 clusters upgraded their pre-v1 Corrosion.
package corromigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/psviderski/uncloud/internal/fs"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/store"
	"google.golang.org/protobuf/encoding/protojson"
	_ "modernc.org/sqlite"
)

// legacyUnitName is the systemd unit that ran Corrosion before uncloudd took over its lifecycle.
// The unit file at /etc/systemd/system/uncloud-corrosion.service and the binary at
// /usr/local/bin/uncloud-corrosion are left in place after migration because uncloudd's sandbox
// (ProtectSystem=full) blocks their removal. Operators should delete them manually as documented
// in the release notes.
const legacyUnitName = "uncloud-corrosion.service"

// Seed is the on-disk representation of the durable rows dumped from a v0.x Corrosion store,
// to be re-applied to the fresh v1.0.0 store. Existence of the seed file is the signal that
// migration has not yet fully completed.
type Seed struct {
	Cluster  []ClusterEntry `json:"cluster"`
	Machines []MachineEntry `json:"machines"`
}

type ClusterEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MachineEntry struct {
	ID   string `json:"id"`
	Info string `json:"info"`
}

func seedPath(dir string) string { return dir + ".seed-v1.json" }

func backupPath(dir string) string {
	return dir + ".backup-" + time.Now().UTC().Format("20060102.150405")
}

// MigrateIfNeeded stops the legacy systemd Corrosion unit and, when a v0.x store.db is detected, dumps its durable
// rows to a seed file next to the data dir, then backs up the dir so the new version can start fresh.
// Idempotent: if the seed file already exists, the dump step is skipped (a prior run already produced it).
func MigrateIfNeeded(ctx context.Context, dir, owner string) error {
	stopLegacyUnit()

	seedFile := seedPath(dir)
	if _, err := os.Stat(seedFile); err == nil {
		slog.Info("Corrosion migrations seed file found, dump already produced by a prior run.",
			"path", seedFile)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat seed file '%s': %w", seedFile, err)
	}

	dbFile := filepath.Join(dir, "store.db")
	if _, err := os.Stat(dbFile); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat '%s': %w", dbFile, err)
	}

	isV0, err := isV0Store(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("detect old store version: %w", err)
	}
	if !isV0 {
		return nil
	}

	slog.Info("Migrating Corrosion store from v0.x to v1.0.0.", "db", dbFile)

	dump, err := dumpOldStore(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("dump old store: %w", err)
	}
	slog.Info("Corrosion store dumped.",
		"cluster_rows", len(dump.Cluster), "machine_rows", len(dump.Machines))

	if err = writeSeedAtomic(seedFile, dump, owner); err != nil {
		return fmt.Errorf("write seed file: %w", err)
	}
	slog.Info("Corrosion migration seed file written.", "path", seedFile)

	backup := backupPath(dir)
	if err = os.Rename(dir, backup); err != nil {
		return fmt.Errorf("backup corrosion dir: %w", err)
	}
	slog.Info("Old Corrosion dir backed up.", "from", dir, "to", backup)

	return nil
}

// stopLegacyUnit stops and disables the legacy uncloud-corrosion systemd unit if present.
// Best-effort: errors are swallowed because the unit may not exist (fresh install) or systemctl
// may be unavailable (containerised hosts).
func stopLegacyUnit() {
	for _, cmd := range []string{"stop", "disable"} {
		if err := exec.Command("systemctl", cmd, legacyUnitName).Run(); err != nil {
			slog.Debug("systemctl on legacy corrosion unit failed (likely absent).",
				"cmd", cmd, "unit", legacyUnitName, "err", err)
		}
	}
}

// isV0Store reports whether the SQLite database at dbPath is in the old v0.x Corrosion format.
// v0.x has a __corro_bookkeeping table that was dropped in v1.0.0.
func isV0Store(ctx context.Context, dbPath string) (bool, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return false, fmt.Errorf("open db '%s': %w", dbPath, err)
	}
	defer db.Close()

	var one int
	err = db.QueryRowContext(ctx,
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='__corro_bookkeeping' LIMIT 1",
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func dumpOldStore(ctx context.Context, dbPath string) (*Seed, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbPath, err)
	}
	defer db.Close()

	seed := &Seed{}

	clusterRows, err := db.QueryContext(ctx, "SELECT key, value FROM cluster")
	if err != nil {
		return nil, fmt.Errorf("query cluster: %w", err)
	}
	for clusterRows.Next() {
		var key, value string
		if err = clusterRows.Scan(&key, &value); err != nil {
			clusterRows.Close()
			return nil, fmt.Errorf("scan cluster row: %w", err)
		}
		seed.Cluster = append(seed.Cluster, ClusterEntry{Key: key, Value: value})
	}
	if err = clusterRows.Err(); err != nil {
		clusterRows.Close()
		return nil, fmt.Errorf("iterate cluster rows: %w", err)
	}
	clusterRows.Close()

	machineRows, err := db.QueryContext(ctx, "SELECT id, info FROM machines")
	if err != nil {
		return nil, fmt.Errorf("query machines: %w", err)
	}
	defer machineRows.Close()
	for machineRows.Next() {
		var id, info string
		if err = machineRows.Scan(&id, &info); err != nil {
			return nil, fmt.Errorf("scan machine row: %w", err)
		}
		seed.Machines = append(seed.Machines, MachineEntry{ID: id, Info: info})
	}
	if err = machineRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machine rows: %w", err)
	}
	return seed, nil
}

func writeSeedAtomic(path string, seed *Seed, owner string) error {
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write seed: %w", err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("fsync seed: %w", err)
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close seed: %w", err)
	}

	if owner != "" {
		if chErr := fs.Chown(tmp, owner, owner); chErr != nil {
			os.Remove(tmp)
			return chErr
		}
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename seed: %w", err)
	}

	// Fsync the parent directory so the rename entry is durable.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open parent dir for fsync: %w", err)
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return fmt.Errorf("fsync parent dir: %w", err)
	}
	return nil
}

// ApplySeedIfPresent re-applies the durable rows from the seed file into the running Corrosion store,
// then deletes the seed file as the final completion marker. Idempotent: re-running on the same seed
// is a no-op for cluster rows (INSERT OR REPLACE) and machines rows (skipped via GetMachine).
func ApplySeedIfPresent(ctx context.Context, dir string, st *store.Store) error {
	seedFile := seedPath(dir)
	data, err := os.ReadFile(seedFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}

	var seed Seed
	if err = json.Unmarshal(data, &seed); err != nil {
		return fmt.Errorf("parse seed file: %w", err)
	}
	slog.Info("Applying Corrosion migration seed.",
		"cluster_rows", len(seed.Cluster), "machine_rows", len(seed.Machines))

	for _, e := range seed.Cluster {
		if err = st.Put(ctx, e.Key, e.Value); err != nil {
			return fmt.Errorf("seed cluster row '%s': %w", e.Key, err)
		}
	}

	for _, e := range seed.Machines {
		var m pb.MachineInfo
		if err = protojson.Unmarshal([]byte(e.Info), &m); err != nil {
			return fmt.Errorf("parse machine '%s' info: %w", e.ID, err)
		}
		if _, err = st.GetMachine(ctx, m.Id); err == nil {
			continue
		} else if !errors.Is(err, store.ErrMachineNotFound) {
			return fmt.Errorf("check machine '%s': %w", m.Id, err)
		}
		if err = st.CreateMachine(ctx, &m); err != nil {
			return fmt.Errorf("seed machine '%s': %w", m.Id, err)
		}
	}

	if err = os.Remove(seedFile); err != nil {
		return fmt.Errorf("delete seed file: %w", err)
	}
	slog.Info("Corrosion migration completed.")
	return nil
}
