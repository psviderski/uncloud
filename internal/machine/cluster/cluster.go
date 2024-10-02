package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"log/slog"
	"net/netip"
	"time"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/network"
	"uncloud/internal/machine/store"
	"uncloud/internal/secret"
)

type Cluster struct {
	pb.UnimplementedClusterServer

	state *State
	store *store.Store

	// TODO: temporary channel until the state is replaced with networkDB.
	newMachinesCh chan *pb.MachineInfo
}

func NewCluster(state *State, store *store.Store) *Cluster {
	return &Cluster{
		state:         state,
		store:         store,
		newMachinesCh: make(chan *pb.MachineInfo, 1),
	}
}

func (c *Cluster) Init(ctx context.Context, network netip.Prefix) error {
	initialised, err := c.Initialised(ctx)
	if err != nil {
		return err
	}
	if initialised {
		return fmt.Errorf("cluster already initialized")
	}

	if err = c.store.Put(ctx, "network", network.String()); err != nil {
		return fmt.Errorf("put network to store: %w", err)
	}
	if err = c.store.Put(ctx, "created_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("put created_at to store: %w", err)
	}
	return nil
}

func (c *Cluster) Initialised(ctx context.Context) (bool, error) {
	var createdAt string
	if err := c.store.Get(ctx, "created_at", &createdAt); err != nil {
		if errors.Is(err, store.ErrKeyNotFound) {
			return false, nil
		}
		return false, status.Errorf(codes.Internal, "get created_at from store: %v", err)
	}
	return true, nil
}

func (c *Cluster) checkInitialised(ctx context.Context) error {
	initialised, err := c.Initialised(ctx)
	if err != nil {
		return err
	}
	if !initialised {
		return status.Error(codes.FailedPrecondition, "cluster is not initialized")
	}
	return nil
}

func (c *Cluster) SetState(state *State) {
	c.state = state
}

func (c *Cluster) Network(ctx context.Context) (netip.Prefix, error) {
	if err := c.checkInitialised(ctx); err != nil {
		return netip.Prefix{}, err
	}

	var net string
	if err := c.store.Get(ctx, "network", &net); err != nil {
		return netip.Prefix{}, status.Errorf(codes.Internal, "get network from store: %v", err)
	}
	prefix, err := netip.ParsePrefix(net)
	if err != nil {
		return netip.Prefix{}, status.Errorf(codes.Internal, "parse network prefix: %v", err)
	}
	return prefix, nil
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
	if err := c.checkInitialised(ctx); err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(req.Network.Endpoints) == 0 {
		return nil, status.Error(codes.InvalidArgument, "endpoints not set")
	}

	machines, err := c.store.ListMachines(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list machines: %v", err)
	}
	allocatedSubnets := make([]netip.Prefix, len(machines))
	for i, m := range machines {
		if req.Name != "" && m.Name == req.Name {
			return nil, status.Errorf(codes.AlreadyExists, "machine with name %q already exists", req.Name)
		}
		if req.Network.ManagementIp != nil && req.Network.ManagementIp.Equal(m.Network.ManagementIp) {
			manageIP, _ := req.Network.ManagementIp.ToAddr()
			return nil, status.Errorf(codes.AlreadyExists, "machine with management IP %q already exists", manageIP)
		}
		if bytes.Equal(m.Network.PublicKey, req.Network.PublicKey) {
			publicKey := secret.Secret(m.Network.PublicKey)
			return nil, status.Errorf(codes.AlreadyExists, "machine with public key %q already exists", publicKey)
		}
		allocatedSubnets[i], _ = m.Network.Subnet.ToPrefix()
	}

	mid, err := NewMachineID()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate machine ID: %v", err)
	}
	name := req.Name
	if name == "" {
		if name, err = NewRandomMachineName(); err != nil {
			return nil, status.Errorf(codes.Internal, "generate machine name: %v", err)
		}
	}
	manageIP := req.Network.ManagementIp
	if manageIP == nil {
		manageIP = pb.NewIP(network.ManagementIP(req.Network.PublicKey))
	}
	// Allocate a subnet for the machine from the cluster network.
	clusterNetwork, err := c.Network(ctx)
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

	m := &pb.MachineInfo{
		Id:   mid,
		Name: name,
		Network: &pb.NetworkConfig{
			Subnet:       pb.NewIPPrefix(subnet),
			ManagementIp: manageIP,
			PublicKey:    req.Network.PublicKey,
		},
	}

	// TODO: announce the new machine to the cluster members and achieve consensus.
	//  We should perhaps not proceed if this machine is in a minority partition.
	if err = c.store.CreateMachine(ctx, m); err != nil {
		return nil, status.Errorf(codes.Internal, "create machine: %v", err)
	}
	slog.Info("Machine added to the cluster.",
		"id", m.Id, "name", m.Name, "subnet", subnet, "public_key", secret.Secret(m.Network.PublicKey))

	// TODO: Subscribe all cluster members to updates about the new machine so they can update their peers config.
	//  In PoC we just notify the local machine.
	c.newMachinesCh <- m

	resp := &pb.AddMachineResponse{Machine: m}
	return resp, nil
}

func (c *Cluster) ListMachines(ctx context.Context, _ *emptypb.Empty) (*pb.ListMachinesResponse, error) {
	if err := c.checkInitialised(ctx); err != nil {
		return nil, err
	}

	machines, err := c.store.ListMachines(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.ListMachinesResponse{Machines: machines}, nil
}

func (c *Cluster) AddUser(user *pb.User) error {
	c.state.State.Users = append(c.state.State.Users, user)
	return c.state.Save()
}

func (c *Cluster) ListUsers() []*pb.User {
	return c.state.State.Users
}
