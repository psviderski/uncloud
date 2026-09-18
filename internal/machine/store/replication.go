package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidStoreVersion indicates an invalid actor UUID in a store version vector.
var ErrInvalidStoreVersion = errors.New("invalid store version")

// WaitForVersion waits until the local store has reached each actor's minimum version in minVersion, with no known
// missing or pending transactions through those versions. Corrosion may satisfy a version by applying its surviving
// changes or by marking it complete because its changes have been superseded.
//
// Waiting normally makes the captured data available locally. However, if another write replaces some of that data
// before it arrives, Corrosion can complete the older version without transferring the replaced data. If the
// replacement is outside minVersion, this method can return while the affected data is still missing or outdated.
// This can happen during concurrent updates even when all machines are well connected.
//
// This method does not guarantee an exact snapshot or delivery of every historical value.
// Callers that require a specific record or condition should verify it after waiting.
//
// The method observes native replication without initiating synchronisation.
// An empty minVersion requires no replication. The context controls cancellation and the deadline.
func (s *Store) WaitForVersion(ctx context.Context, minVersion map[string]uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	placeholders := make([]string, 0, len(minVersion))
	args := make([]any, 0, 2*len(minVersion))
	for actor, version := range minVersion {
		actorID, err := uuid.Parse(actor)
		if err != nil {
			return fmt.Errorf("%w: actor '%s' is not a UUID: %v", ErrInvalidStoreVersion, actor, err)
		}
		if version == 0 {
			continue
		}

		// Corrosion accepts a JSON array of bytes as a BLOB parameter. A UUID marshals as a string,
		// and a byte slice marshals as base64, so use a plain fixed-size array instead.
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, [16]byte(actorID), version)
	}
	if len(args) == 0 {
		return nil
	}

	// Check all bookkeeping in one SQLite snapshot. Raw buffered rows can remain after application,
	// but sequence bookkeeping is removed in the same transaction that applies or clears a version.
	query := `WITH min_version(actor_id, version) AS (VALUES ` + strings.Join(placeholders, ", ") + `)
		SELECT NOT EXISTS (
			SELECT 1 FROM min_version
			LEFT JOIN crsql_db_versions AS current ON current.site_id = min_version.actor_id
			WHERE COALESCE(current.db_version, 0) < min_version.version
				OR EXISTS (
					SELECT 1 FROM __corro_bookkeeping_gaps
					WHERE actor_id = min_version.actor_id AND start <= min_version.version
				)
				OR EXISTS (
					SELECT 1 FROM __corro_seq_bookkeeping
					WHERE site_id = min_version.actor_id AND db_version <= min_version.version
				)
		)`

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		rows, err := s.corro.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("check store replication: %w", err)
		}
		if !rows.Next() {
			if err = rows.Err(); err != nil {
				return fmt.Errorf("check store replication: %w", err)
			}
			return errors.New("check store replication: store version query returned no rows")
		}
		var reached int
		if err = rows.Scan(&reached); err != nil {
			rows.Close()
			return fmt.Errorf("check store replication: %w", err)
		}
		rows.Next() // Consume the end-of-query event.
		rows.Close()
		if err = rows.Err(); err != nil {
			return fmt.Errorf("check store replication: %w", err)
		}
		if reached == 1 {
			return ctx.Err()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
