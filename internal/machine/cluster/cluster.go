package cluster

import (
	"bytes"
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"net/netip"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
	"uncloud/internal/secret"
)

type Cluster struct {
	pb.UnimplementedClusterServer

	state *State

	// TODO: temporary channel until the state is replaced with networkDB.
	newMachinesCh chan *pb.MachineInfo
}

func NewCluster(state *State) *Cluster {
	return &Cluster{
		state:         state,
		newMachinesCh: make(chan *pb.MachineInfo),
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

// TODO: this is a temporary watcher for PoC until the state is state is replaced with networkDB.
func (c *Cluster) WatchNewMachines() <-chan *pb.MachineInfo {
	return c.newMachinesCh
}

// AddMachine adds a machine to the cluster.
func (c *Cluster) AddMachine(ctx context.Context, req *pb.AddMachineRequest) (*pb.AddMachineResponse, error) {
	if c.state == nil {
		return nil, status.Error(codes.FailedPrecondition, "cluster is not initialized")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(req.Network.Endpoints) == 0 {
		return nil, status.Error(codes.InvalidArgument, "endpoints not set")
	}

	machines := c.state.State.Machines
	allocatedSubnets := make([]netip.Prefix, len(machines))
	var err error
	i := 0
	for _, m := range machines {
		if req.Name != "" && m.Name == req.Name {
			return nil, status.Errorf(codes.AlreadyExists, "machine with name %q already exists", req.Name)
		}
		if m.Network.ManagementIp != nil && req.Network.ManagementIp != nil &&
			m.Network.ManagementIp.Equal(req.Network.ManagementIp) {
			manageIP, _ := req.Network.ManagementIp.ToAddr()
			return nil, status.Errorf(codes.AlreadyExists, "machine with management IP %q already exists", manageIP)
		}
		if bytes.Equal(m.Network.PublicKey, req.Network.PublicKey) {
			publicKey := secret.Secret(m.Network.PublicKey)
			return nil, status.Errorf(codes.AlreadyExists, "machine with public key %q already exists", publicKey)
		}

		if allocatedSubnets[i], err = m.Network.Subnet.ToPrefix(); err != nil {
			return nil, err
		}
		i++
	}

	mid, err := NewMachineID()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate machine ID: %v", err)
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
			return nil, status.Errorf(codes.Internal, "generate machine name: %v", err)
		}
	}

	clusterNetwork, err := c.Network()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster network: %v", err)
	}
	ipam, err := NewIPAMWithAllocated(clusterNetwork, allocatedSubnets)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create IPAM manager: %v", err)
	}
	subnet, err := ipam.AllocateSubnetLen(network.DefaultSubnetBits)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "allocate subnet for machine: %v", err)
	}
	m.Network.Subnet = pb.NewIPPrefix(subnet)

	// TODO: announce the new machine to the cluster network using Serf and achieve consensus.
	mCopy := proto.Clone(m).(*pb.MachineInfo)
	c.state.State.Machines[mCopy.Id] = mCopy
	// Store machine endpoints in a separate collection to allow modifying them with limited permissions.
	c.state.State.Endpoints[mCopy.Id] = &pb.MachineEndpoints{
		Id:        mCopy.Id,
		Endpoints: req.Network.Endpoints,
	}
	if err = c.state.Save(); err != nil {
		return nil, status.Errorf(codes.Internal, "save state: %v", err)
	}
	slog.Info("Machine added to the cluster.", "id", m.Id, "name", m.Name)

	// Include the machine endpoints in the response.
	m.Network.Endpoints = req.Network.Endpoints

	// TODO: notify all cluster machines about the new machine so they can update their peers config.
	//  In PoC we just notify the local machine.
	c.newMachinesCh <- m

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
