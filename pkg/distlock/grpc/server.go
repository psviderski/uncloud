// Package grpc provides a gRPC transport for distlock node-local lease operations.
package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/psviderski/uncloud/pkg/distlock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Server adapts a node-local distlock.Store to the Lease gRPC service.
type Server struct {
	UnimplementedLeaseServer
	store distlock.Store
}

// NewServer creates a node-local lease server.
func NewServer(store distlock.Store) *Server {
	return &Server{store: store}
}

// Acquire creates a lease when the resource has no unexpired lease.
func (s *Server) Acquire(ctx context.Context, req *AcquireLeaseRequest) (*AcquireLeaseResponse, error) {
	ttl, err := validateLeaseRequest(req.Resource, req.Token, req.Ttl)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	acquired, err := s.store.Acquire(ctx, req.Resource, req.Token, ttl)
	if err != nil {
		return nil, storeStatusError(ctx, "acquire lease", err)
	}
	return &AcquireLeaseResponse{Acquired: acquired}, nil
}

// Renew extends an unexpired lease when its ownership token matches.
func (s *Server) Renew(ctx context.Context, req *RenewLeaseRequest) (*RenewLeaseResponse, error) {
	ttl, err := validateLeaseRequest(req.Resource, req.Token, req.Ttl)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	renewed, err := s.store.Renew(ctx, req.Resource, req.Token, ttl)
	if err != nil {
		return nil, storeStatusError(ctx, "renew lease", err)
	}
	return &RenewLeaseResponse{Renewed: renewed}, nil
}

// Release removes an unexpired lease when its ownership token matches.
func (s *Server) Release(ctx context.Context, req *ReleaseLeaseRequest) (*ReleaseLeaseResponse, error) {
	if err := validateResourceToken(req.Resource, req.Token); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	released, err := s.store.Release(ctx, req.Resource, req.Token)
	if err != nil {
		return nil, storeStatusError(ctx, "release lease", err)
	}
	return &ReleaseLeaseResponse{Released: released}, nil
}

func validateLeaseRequest(resource string, token []byte, ttl *durationpb.Duration) (time.Duration, error) {
	if err := validateResourceToken(resource, token); err != nil {
		return 0, err
	}
	if ttl == nil {
		return 0, fmt.Errorf("TTL is not set")
	}
	if err := ttl.CheckValid(); err != nil {
		return 0, fmt.Errorf("invalid TTL: %w", err)
	}
	duration := ttl.AsDuration()
	if duration <= 0 {
		return 0, fmt.Errorf("TTL must be positive")
	}
	return duration, nil
}

func validateResourceToken(resource string, token []byte) error {
	if resource == "" {
		return fmt.Errorf("resource is empty")
	}
	if len(token) == 0 {
		return fmt.Errorf("token is empty")
	}
	return nil
}

func storeStatusError(ctx context.Context, operation string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return status.FromContextError(ctxErr).Err()
	}
	return status.Error(codes.Internal, fmt.Sprintf("%s: %v", operation, err))
}
