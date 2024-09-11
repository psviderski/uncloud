package cluster

import (
	"bytes"
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"net/netip"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
)

type Cluster struct {
	pb.UnimplementedClusterServer

	state *State
}

func NewCluster(state *State) *Cluster {
	return &Cluster{
		state: state,
	}
}

func (c *Cluster) SetState(state *State) {
	c.state = state
}

func (c *Cluster) Network() (netip.Prefix, error) {
	if c.state == nil {
		return netip.Prefix{}, status.Error(codes.FailedPrecondition, "cluster is not initialized")
	}

	if c.state.State.Network == nil {
		return netip.Prefix{}, fmt.Errorf("network not set")
	}
	return c.state.State.Network.ToPrefix()
}

func (c *Cluster) SetNetwork(network *pb.IPPrefix) error {
	if c.state == nil {
		return status.Error(codes.FailedPrecondition, "cluster is not initialized")
	}

	if c.state.State.Network != nil {
		return fmt.Errorf("network already set and cannot be changed")
	}
	if network == nil {
		return fmt.Errorf("network not set")
	}
	c.state.State.Network = network
	return nil
}

// AddMachine adds a machine to the cluster.
func (c *Cluster) AddMachine(ctx context.Context, req *pb.AddMachineRequest) (*pb.AddMachineResponse, error) {
	if c.state == nil {
		return nil, status.Error(codes.FailedPrecondition, "cluster is not initialized")
	}

	// TODO: replace errors with gRPC status.Error(f), e.g. status.Error(codes.InvalidArgument, "management IP not set")
	if req.Network.PublicKey == nil {
		return nil, fmt.Errorf("public key not set")
	}

	machines := c.state.State.Machines
	allocatedSubnets := make([]netip.Prefix, len(machines))
	var err error
	i := 0
	for _, m := range machines {
		if req.Name != "" && m.Name == req.Name {
			return nil, fmt.Errorf("machine with name %q already exists", req.Name)
		}
		if m.Network.ManagementIp != nil && m.Network.ManagementIp.Equal(req.Network.ManagementIp) {
			return nil, fmt.Errorf("machine with management IP %q already exists", req.Network.ManagementIp)
		}
		if bytes.Equal(m.Network.PublicKey, req.Network.PublicKey) {
			return nil, fmt.Errorf("machine with public key %q already exists", req.Network.PublicKey)
		}

		if allocatedSubnets[i], err = m.Network.Subnet.ToPrefix(); err != nil {
			return nil, err
		}
		i++
	}

	mid, err := NewMachineID()
	if err != nil {
		return nil, fmt.Errorf("generate machine ID: %w", err)
	}
	manageIP := req.Network.ManagementIp
	if manageIP == nil {
		manageIP = pb.NewIP(network.ManagementIP(req.Network.PublicKey))
	}
	m := &pb.MachineInfo{
		Id:   mid,
		Name: req.Name,
		Network: &pb.NetworkConfig{
			ManagementIp: manageIP,
			PublicKey:    req.Network.PublicKey,
		},
	}
	if m.Name == "" {
		m.Name, err = NewRandomMachineName()
		if err != nil {
			return nil, fmt.Errorf("generate machine name: %w", err)
		}
	}

	clusterNetwork, err := c.Network()
	if err != nil {
		return nil, fmt.Errorf("get cluster network: %w", err)
	}
	ipam, err := NewIPAMWithAllocated(clusterNetwork, allocatedSubnets)
	if err != nil {
		return nil, fmt.Errorf("create IPAM manager: %w", err)
	}
	subnet, err := ipam.AllocateSubnetLen(network.DefaultSubnetBits)
	if err != nil {
		return nil, fmt.Errorf("allocate subnet: %w", err)
	}
	m.Network.Subnet = pb.NewIPPrefix(subnet)

	// TODO: announce the new machine to the cluster network using Serf and achieve consensus.
	mState := proto.Clone(m).(*pb.MachineInfo)
	c.state.State.Machines[mState.Id] = mState
	// Store machine endpoints in a separate collection to allow modifying them with limited permissions.
	c.state.State.Endpoints[mState.Id] = &pb.MachineEndpoints{
		Id:        mState.Id,
		Endpoints: req.Network.Endpoints,
	}
	if err = c.state.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save state: %v", err)
	}

	// Include the machine endpoints in the response.
	m.Network.Endpoints = req.Network.Endpoints

	resp := &pb.AddMachineResponse{Machine: m}
	return resp, nil
}

func (c *Cluster) ListMachineEndpoints(
	ctx context.Context, req *pb.ListMachineEndpointsRequest,
) (*pb.ListMachineEndpointsResponse, error) {
	if c.state == nil {
		return nil, status.Error(codes.FailedPrecondition, "cluster is not initialized")
	}

	endpoints, ok := c.state.State.Endpoints[req.Id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "machine %q not found", req.Id)
	}
	return &pb.ListMachineEndpointsResponse{Endpoints: endpoints}, nil
}

func (c *Cluster) AddUser(user *pb.User) error {
	c.state.State.Users = append(c.state.State.Users, user)
	return c.state.Save()
}

func (c *Cluster) ListUsers() []*pb.User {
	return c.state.State.Users
}
