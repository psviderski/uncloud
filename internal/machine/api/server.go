package api

import (
	"context"
	pb2 "uncloud/internal/machine/api/pb"
)

// Server is the gRPC server for the Cluster service.
type Server struct {
	pb2.UnimplementedClusterServer
}

func NewServer() *Server {
	return &Server{}
}

// AddMachine adds a machine to the cluster.
func (s *Server) AddMachine(ctx context.Context, req *pb2.AddMachineRequest) (*pb2.AddMachineResponse, error) {
	return &pb2.AddMachineResponse{}, nil
}
