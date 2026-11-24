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

	rows, err := k.corro.QueryContext(ctx, "SELECT key, value, updated_at FROM cluster WHERE key = ?",
		k.namespacedKey(key))
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

	return k.scanRecord(rows.Scan)
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

// KeyspaceDeleteOptions controls which records [Keyspace.Delete] deletes.
type KeyspaceDeleteOptions struct {
	// Prefix deletes the key and every key that starts with it.
	Prefix bool
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

// KeyspaceListOptions controls the records returned by [Keyspace.List].
type KeyspaceListOptions struct {
	// KeysOnly omits record values from the query when true.
	KeysOnly bool
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

// KeyspaceSubscribeOptions controls which records [Keyspace.Subscribe] watches.
type KeyspaceSubscribeOptions struct {
	// Prefix watches the key and every key that starts with it. An empty key watches the whole keyspace.
	Prefix bool
}

// KeyEvent is the state of the record at a key that [Keyspace.Subscribe] sends.
type KeyEvent struct {
	// Key is the key the event is for. It's empty for a subscription error, or if the key of a record couldn't
	// be read.
	Key string
	// Record is the record at Key. It's nil if the key was deleted, or Err is set.
	Record *Record
	// Err is set if the record couldn't be read or the subscription failed. The subscription continues after
	// a record error and ends after a subscription error.
	Err error
}

// Subscribe returns an event for each record at key, ordered by key, and a channel that receives an event every
// time one of the records is put or deleted. The channel is closed when ctx is cancelled or after an event with
// a subscription error.
func (k *Keyspace) Subscribe(
	ctx context.Context, key string, opts KeyspaceSubscribeOptions,
) ([]KeyEvent, <-chan KeyEvent, error) {
	if key == "" && !opts.Prefix {
		return nil, nil, fmt.Errorf("key is empty")
	}

	namespacedKey := k.namespacedKey(key)
	query := "SELECT key, value, updated_at FROM cluster WHERE key = ?"
	args := []any{namespacedKey}
	if opts.Prefix {
		query = "SELECT key, value, updated_at FROM cluster WHERE key >= ? AND key < ? ORDER BY key"
		args = []any{namespacedKey, prefixRangeEnd(namespacedKey)}
	}

	sub, err := k.corro.SubscribeContext(ctx, query, args, false)
	if err != nil {
		return nil, nil, err
	}

	// Delete events can only be matched to keys by row ID.
	keys := make(map[uint64]string)
	var current []KeyEvent
	rows := sub.Rows()
	for rows.Next() {
		event := k.recordEvent(rows.Scan)
		if event.Key != "" {
			keys[rows.RowID()] = event.Key
		}
		current = append(current, event)
	}
	if err = rows.Err(); err != nil {
		sub.Close()
		return nil, nil, fmt.Errorf("read records: %w", err)
	}

	changes, err := sub.Changes()
	if err != nil {
		sub.Close()
		return nil, nil, fmt.Errorf("get subscription changes: %w", err)
	}

	events := make(chan KeyEvent)
	go func() {
		defer close(events)

		for {
			var event KeyEvent
			select {
			case change, ok := <-changes:
				if !ok {
					// The changes channel is closed without an error only when ctx is cancelled.
					if sErr := sub.Err(); sErr != nil {
						select {
						case events <- KeyEvent{Err: fmt.Errorf("subscription %s failed: %w", sub.ID(), sErr)}:
						case <-ctx.Done():
						}
					}
					return
				}

				if change.Type == corrosion.ChangeTypeDelete {
					if deletedKey, known := keys[change.RowID]; known {
						delete(keys, change.RowID)
						event = KeyEvent{Key: deletedKey}
					} else {
						event = KeyEvent{Err: fmt.Errorf("deleted row %d has an unknown key", change.RowID)}
					}
				} else {
					event = k.recordEvent(change.Scan)
					if event.Key != "" {
						keys[change.RowID] = event.Key
					}
				}
			case <-ctx.Done():
				return
			}

			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return current, events, nil
}

// recordEvent returns an event with the record read from the key, value and updated_at columns.
func (k *Keyspace) recordEvent(scan func(dest ...any) error) KeyEvent {
	record, err := k.scanRecord(scan)
	if err != nil {
		return KeyEvent{Key: record.Key, Err: err}
	}
	return KeyEvent{Key: record.Key, Record: &record}
}

// scanRecord reads a record from the key, value and updated_at columns. If it returns an error, the record only
// has the key if it could be read.
func (k *Keyspace) scanRecord(scan func(dest ...any) error) (Record, error) {
	var (
		record    Record
		updatedAt string
	)
	err := scan(&record.Key, &record.Value, &updatedAt)
	record.Key = strings.TrimPrefix(record.Key, k.namespacedKey(""))
	if err != nil {
		return Record{Key: record.Key}, fmt.Errorf("scan record: %w", err)
	}

	if record.UpdatedAt, err = parseTimestamp(updatedAt); err != nil {
		return Record{Key: record.Key}, fmt.Errorf("parse updated_at for key '%s': %w", record.Key, err)
	}
	return record, nil
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
