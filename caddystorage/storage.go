package caddystorage

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/caddyserver/certmagic"
	"github.com/psviderski/uncloud/api/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Store writes a value to the cluster store.
func (s *Storage) Store(ctx context.Context, key string, value []byte) error {
	if _, err := s.client.CaddyStorage.Store(ctx, &pb.StoreCaddyStorageRequest{Key: key, Value: value}); err != nil {
		return storageError("store", key, err)
	}
	return nil
}

// Load reads a value from the cluster store.
func (s *Storage) Load(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.client.CaddyStorage.Load(ctx, &pb.LoadCaddyStorageRequest{Key: key})
	if err != nil {
		return nil, storageError("load", key, err)
	}
	return resp.Value, nil
}

// Delete removes a key and its descendants from the cluster store.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if _, err := s.client.CaddyStorage.Delete(ctx, &pb.DeleteCaddyStorageRequest{Key: key}); err != nil {
		return storageError("delete", key, err)
	}
	return nil
}

// Exists reports whether a key exists in the cluster store.
func (s *Storage) Exists(ctx context.Context, key string) bool {
	_, err := s.Stat(ctx, key)
	return err == nil
}

// List returns keys under prefix from the cluster store.
func (s *Storage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	resp, err := s.client.CaddyStorage.List(ctx, &pb.ListCaddyStorageRequest{
		Prefix:    prefix,
		Recursive: recursive,
	})
	if err != nil {
		return nil, storageError("list", prefix, err)
	}
	return resp.Keys, nil
}

// Stat returns information about a key in the cluster store.
func (s *Storage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	resp, err := s.client.CaddyStorage.Stat(ctx, &pb.StatCaddyStorageRequest{Key: key})
	if err != nil {
		return certmagic.KeyInfo{}, storageError("stat", key, err)
	}

	info := certmagic.KeyInfo{
		Key:        resp.Key,
		Size:       resp.Size,
		IsTerminal: resp.IsTerminal,
	}
	if resp.UpdatedAt != nil {
		if err := resp.UpdatedAt.CheckValid(); err != nil {
			return certmagic.KeyInfo{}, storageError("stat", key, fmt.Errorf("invalid updated_at timestamp: %w", err))
		}
		info.Modified = resp.UpdatedAt.AsTime()
	}
	return info, nil
}

func storageError(operation, key string, err error) error {
	if status.Code(err) == codes.NotFound {
		err = fs.ErrNotExist
	}
	return fmt.Errorf("uncloud storage: %s key %q: %w", operation, key, err)
}
