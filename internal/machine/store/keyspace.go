package store

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/psviderski/uncloud/internal/corrosion"
)

const namespaceSeparator = ":"

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[_-][a-z0-9]+)*$`)

// Record is a key-value entry and its metadata.
type Record struct {
	Key       string
	Value     []byte
	UpdatedAt time.Time
}

// KeyspaceListOptions controls the records returned by [Keyspace.List].
type KeyspaceListOptions struct {
	// KeysOnly omits record values from the query when true.
	KeysOnly bool
}

// KeyspaceDeleteOptions controls which records [Keyspace.Delete] deletes.
type KeyspaceDeleteOptions struct {
	// Prefix deletes the key and every key that starts with it.
	Prefix bool
}

// Keyspace provides namespace-scoped key-value storage.
// Keys are relative to the namespace. Prefix operations use simple string-prefix matching.
type Keyspace struct {
	corro     *corrosion.APIClient
	namespace string
}

// Keyspace returns key-value storage scoped to namespace.
func (s *Store) Keyspace(namespace string) (*Keyspace, error) {
	if !namespacePattern.MatchString(namespace) {
		return nil, fmt.Errorf("invalid namespace %q", namespace)
	}
	return &Keyspace{
		corro:     s.corro,
		namespace: namespace,
	}, nil
}

// Get returns the entry for key. It returns [ErrKeyNotFound] when the key does not exist.
func (k *Keyspace) Get(ctx context.Context, key string) (Record, error) {
	if key == "" {
		return Record{}, fmt.Errorf("key is empty")
	}

	rows, err := k.corro.QueryContext(ctx, "SELECT value, updated_at FROM cluster WHERE key = ?", k.namespacedKey(key))
	if err != nil {
		return Record{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return Record{}, err
		}
		return Record{}, ErrKeyNotFound
	}

	record := Record{Key: key}
	var updatedAt string
	if err = rows.Scan(&record.Value, &updatedAt); err != nil {
		return Record{}, err
	}
	if record.UpdatedAt, err = parseTimestamp(updatedAt); err != nil {
		return Record{}, fmt.Errorf("parse updated_at for key %q: %w", key, err)
	}
	return record, nil
}

// Put stores value at key.
func (k *Keyspace) Put(ctx context.Context, key string, value []byte) error {
	if key == "" {
		return fmt.Errorf("key is empty")
	}

	_, err := k.corro.ExecContext(ctx,
		"INSERT OR REPLACE INTO cluster (key, value, updated_at) VALUES (?, ?, datetime('now', 'subsec'))",
		k.namespacedKey(key), value)
	return err
}

// Delete deletes key. When Prefix is true, it also deletes every key that starts with key.
func (k *Keyspace) Delete(ctx context.Context, key string, opts KeyspaceDeleteOptions) error {
	if key == "" {
		return fmt.Errorf("key is empty")
	}

	namespacedKey := k.namespacedKey(key)
	if !opts.Prefix {
		_, err := k.corro.ExecContext(ctx, "DELETE FROM cluster WHERE key = ?", namespacedKey)
		return err
	}

	_, err := k.corro.ExecContext(ctx, `DELETE FROM cluster WHERE key >= ? AND key < ?`,
		namespacedKey, prefixRangeEnd(namespacedKey))
	return err
}

// List returns entries whose keys start with prefix, ordered by key. An empty prefix lists every entry in the keyspace.
func (k *Keyspace) List(ctx context.Context, prefix string, opts KeyspaceListOptions) ([]Record, error) {
	keyspacePrefix := k.namespacedKey("")
	namespacedPrefix := k.namespacedKey(prefix)
	query := "SELECT key, value, updated_at FROM cluster WHERE key >= ? AND key < ? ORDER BY key"
	if opts.KeysOnly {
		query = "SELECT key, updated_at FROM cluster WHERE key >= ? AND key < ? ORDER BY key"
	}

	rows, err := k.corro.QueryContext(ctx, query, namespacedPrefix, prefixRangeEnd(namespacedPrefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var (
			record    Record
			updatedAt string
		)
		if opts.KeysOnly {
			err = rows.Scan(&record.Key, &updatedAt)
		} else {
			err = rows.Scan(&record.Key, &record.Value, &updatedAt)
		}
		if err != nil {
			return nil, err
		}
		if record.UpdatedAt, err = parseTimestamp(updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at for key %q: %w", record.Key, err)
		}
		record.Key = strings.TrimPrefix(record.Key, keyspacePrefix)
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func (k *Keyspace) namespacedKey(key string) string {
	return k.namespace + namespaceSeparator + key
}

func parseTimestamp(value string) (time.Time, error) {
	return time.Parse(time.DateTime, value)
}

// prefixRangeEnd returns an exclusive upper bound for a key prefix.
func prefixRangeEnd(prefix string) string {
	return prefix + string(utf8.MaxRune)
}
