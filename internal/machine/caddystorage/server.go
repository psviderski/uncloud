// Package caddystorage implements the machine-local Caddy storage API.
// See api/pb/caddy_storage.proto for the gRPC service definition.
package caddystorage

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/machine/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Namespace identifies Caddy storage records in the cluster key-value store.
const Namespace = "caddy_storage"

// Server implements the machine-local CaddyStorage gRPC service.
type Server struct {
	pb.UnimplementedCaddyStorageServer
	store *store.Keyspace
}

func NewServer(store *store.Keyspace) *Server {
	return &Server{store: store}
}

func (s *Server) Store(ctx context.Context, req *pb.StoreCaddyStorageRequest) (*emptypb.Empty, error) {
	if err := validateKey(req.Key); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.store.Put(ctx, req.Key, req.Value); err != nil {
		return nil, status.Errorf(codes.Internal, "put value: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) Load(ctx context.Context, req *pb.LoadCaddyStorageRequest) (*pb.LoadCaddyStorageResponse, error) {
	if err := validateKey(req.Key); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	record, err := s.store.Get(ctx, req.Key)
	if errors.Is(err, store.ErrKeyNotFound) {
		return nil, status.Errorf(codes.NotFound, "Caddy storage key %q not found", req.Key)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get value: %v", err)
	}

	return &pb.LoadCaddyStorageResponse{
		Value:     record.Value,
		UpdatedAt: timestamppb.New(record.UpdatedAt),
	}, nil
}

func (s *Server) Delete(ctx context.Context, req *pb.DeleteCaddyStorageRequest) (*emptypb.Empty, error) {
	if err := validateKey(req.Key); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Delete the key and all keys prefixed by it in case it's a directory.
	if err := errors.Join(
		s.store.Delete(ctx, req.Key, store.KeyspaceDeleteOptions{}),
		s.store.Delete(ctx, req.Key+"/", store.KeyspaceDeleteOptions{Prefix: true}),
	); err != nil {
		return nil, status.Errorf(codes.Internal, "delete key: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) List(ctx context.Context, req *pb.ListCaddyStorageRequest) (*pb.ListCaddyStorageResponse, error) {
	if req.Prefix != "" {
		if err := validateKey(req.Prefix); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	storagePrefix := req.Prefix
	if storagePrefix != "" {
		storagePrefix += "/"
	}
	records, err := s.store.List(ctx, storagePrefix, store.KeyspaceListOptions{KeysOnly: true})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list keys: %v", err)
	}
	// No descendants can mean an existing terminal key or a missing path.
	// Check the exact key for non-root prefixes. An empty prefix lists the storage root.
	if len(records) == 0 && req.Prefix != "" {
		if _, err = s.store.Get(ctx, req.Prefix); errors.Is(err, store.ErrKeyNotFound) {
			return nil, status.Errorf(codes.NotFound, "Caddy storage prefix %q not found", req.Prefix)
		} else if err != nil {
			return nil, status.Errorf(codes.Internal, "get prefix value: %v", err)
		}
	}

	return &pb.ListCaddyStorageResponse{
		Keys: listKeys(records, req.Prefix, req.Recursive),
	}, nil
}

func listKeys(records []store.Record, prefix string, recursive bool) []string {
	keys := make(map[string]struct{})
	for _, record := range records {
		relative := record.Key
		if prefix != "" {
			relative = strings.TrimPrefix(record.Key, prefix+"/")
		}
		components := strings.Split(relative, "/")
		if !recursive {
			keys[path.Join(prefix, components[0])] = struct{}{}
			continue
		}

		for i := range components {
			keys[path.Join(prefix, path.Join(components[:i+1]...))] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(keys))
}

func (s *Server) Stat(ctx context.Context, req *pb.StatCaddyStorageRequest) (*pb.StatCaddyStorageResponse, error) {
	if err := validateKey(req.Key); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	record, err := s.store.Get(ctx, req.Key)
	if err == nil {
		return &pb.StatCaddyStorageResponse{
			Key:        req.Key,
			UpdatedAt:  timestamppb.New(record.UpdatedAt),
			Size:       int64(len(record.Value)),
			IsTerminal: true,
		}, nil
	}
	if !errors.Is(err, store.ErrKeyNotFound) {
		return nil, status.Errorf(codes.Internal, "get value: %v", err)
	}

	// A terminal key is not found. Check if there are any keys prefixed by the key to determine if it's a directory.
	records, err := s.store.List(ctx, req.Key+"/", store.KeyspaceListOptions{KeysOnly: true})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list keys: %v", err)
	}
	if len(records) == 0 {
		return nil, status.Errorf(codes.NotFound, "Caddy storage key %q not found", req.Key)
	}

	return &pb.StatCaddyStorageResponse{
		Key:        req.Key,
		IsTerminal: false,
	}, nil
}

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key is empty")
	}
	if strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return fmt.Errorf("key %q must not have a leading or trailing slash", key)
	}
	if strings.Contains(key, "\\") {
		return fmt.Errorf("key %q must use forward slashes", key)
	}
	for component := range strings.SplitSeq(key, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("key %q contains an invalid path component", key)
		}
	}
	return nil
}
