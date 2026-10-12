package caddyconfig

import (
	"context"
	"errors"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/psviderski/uncloud/api/pb"
)

// Server implements the gRPC Caddy service.
type Server struct {
	pb.UnimplementedCaddyServer
	service *Service
}

func NewServer(service *Service) *Server {
	return &Server{service: service}
}

// GetConfig retrieves the saved Caddy configuration and the latest reconciliation error from the machine.
func (s *Server) GetConfig(ctx context.Context, _ *emptypb.Empty) (*pb.GetCaddyConfigResponse, error) {
	caddyfile, modifiedAt, err := s.service.Caddyfile()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if lastErr := s.service.LastReconciliationError(); lastErr != nil {
				return nil, status.Errorf(codes.NotFound, "%v; last Caddy config load failed: %v", err, lastErr)
			}
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	recErr := ""
	if lastErr := s.service.LastReconciliationError(); lastErr != nil {
		recErr = lastErr.Error()
	}
	return &pb.GetCaddyConfigResponse{
		Caddyfile:               caddyfile,
		ModifiedAt:              timestamppb.New(modifiedAt),
		LastReconciliationError: recErr,
	}, nil
}
