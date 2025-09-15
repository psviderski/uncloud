package caddyconfig

import (
	"context"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

// Server implements the gRPC Caddy service.
type Server struct {
	pb.UnimplementedCaddyServer
	service *Service
}

func NewServer(service *Service) *Server {
	return &Server{service: service}
}

// GetConfig retrieves the current Caddy configuration from the machine.
func (s *Server) GetConfig(ctx context.Context, _ *emptypb.Empty) (*pb.GetCaddyConfigResponse, error) {
	caddyfile, modifiedAt, err := s.service.Caddyfile()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetCaddyConfigResponse{
		Caddyfile:  caddyfile,
		ModifiedAt: timestamppb.New(modifiedAt),
	}, nil
}
