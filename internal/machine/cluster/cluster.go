package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/psviderski/uncloud/internal/corrosion"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/internal/secret"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Cluster struct {
	pb.UnimplementedClusterServer

	store      *store.Store
	corroAdmin *corrosion.AdminClient
	// machineID is the ID of the current machine that is running the cluster service.
	machineID string
	// initialised is closed when the machine is configured as a member of a cluster.
	initialised <-chan struct{}
	// ready is closed when the cluster controller has finished starting all components
	// and the machine is ready to serve cluster requests.
	ready <-chan struct{}
}

func NewCluster(store *store.Store, corroAdmin *corrosion.AdminClient, initialised, ready <-chan struct{}) *Cluster {
	return &Cluster{
		store:       store,
		corroAdmin:  corroAdmin,
		initialised: initialised,
		ready:       ready,
	}
}

// UpdateMachineID updates the current machine ID that is running the cluster service.
func (c *Cluster) UpdateMachineID(mid string) {
	c.machineID = mid
}

func (c *Cluster) Init(ctx context.Context, network netip.Prefix) error {
	select {
	case <-c.initialised:
		return fmt.Errorf("cluster is already initialised on this machine")
	default:
	}

	if err := c.store.Put(ctx, "network", network.String()); err != nil {
		return fmt.Errorf("put network to store: %w", err)
	}
	if err := c.store.Put(ctx, "created_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("put created_at to store: %w", err)
	}
	return nil
}

// checkReady checks if the machine is ready to serve cluster requests (store synced, cluster components started).
func (c *Cluster) checkReady() error {
	select {
	case <-c.ready:
		return nil
	default:
		return status.Error(codes.Unavailable, "machine is not ready to serve cluster requests")
	}
}

// AddMachine adds a machine to the cluster.
func (c *Cluster) AddMachine(ctx context.Context, req *pb.AddMachineRequest) (*pb.AddMachineResponse, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	return c.AddMachineWithoutReadyCheck(ctx, req)
}

// AddMachineWithoutReadyCheck adds a machine to the cluster without checking if the cluster is ready.
// This is used internally during cluster initialisation to add the first machine.
func (c *Cluster) AddMachineWithoutReadyCheck(
	ctx context.Context, req *pb.AddMachineRequest,
) (*pb.AddMachineResponse, error) {
	if req.Network == nil {
		return nil, status.Error(codes.InvalidArgument, "network not set")
	}
	if err := req.Network.Validate(); err != nil {
		return nil, err
	}
	if len(req.Network.Endpoints) == 0 {
		return nil, status.Error(codes.InvalidArgument, "endpoints not set")
	}
	if req.PublicIp != nil {
		ip, err := req.PublicIp.ToAddr()
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid public IP: %v", err)
		}
		if !ip.IsValid() {
			return nil, status.Error(codes.InvalidArgument, "invalid public IP")
		}
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
			return nil, status.Errorf(
				codes.AlreadyExists, "machine with management IP %q already exists under the name %q",
				manageIP, m.Name,
			)
		}
		if bytes.Equal(m.Network.PublicKey, req.Network.PublicKey) {
			publicKey := secret.Secret(m.Network.PublicKey)
			return nil, status.Errorf(
				codes.AlreadyExists, "machine with public key %q already exists under the name %q",
				publicKey, m.Name,
			)
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
	clusterNetwork, err := c.network(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster network: %v", err)
	}
	ipam, err := NewIPAMWithAllocated(clusterNetwork, allocatedSubnets)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create IPAM manager: %v", err)
	}
	subnet, err := ipam.AllocateSubnetLen(DefaultSubnetBits)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "allocate subnet for machine: %v", err)
	}

	m := &pb.MachineInfo{
		Id:   mid,
		Name: name,
		Network: &pb.NetworkConfig{
			Subnet:       pb.NewIPPrefix(subnet),
			ManagementIp: manageIP,
			Endpoints:    req.Network.Endpoints,
			PublicKey:    req.Network.PublicKey,
		},
		PublicIp: req.PublicIp,
	}
	// TODO: announce the new machine to the cluster members and achieve consensus.
	//  We should perhaps not proceed if this machine is in a minority partition.
	if err = c.store.CreateMachine(ctx, m); err != nil {
		return nil, status.Errorf(codes.Internal, "create machine: %v", err)
	}
	slog.Info("Machine added to the cluster.",
		"id", m.Id, "name", m.Name, "subnet", subnet, "public_key", secret.Secret(m.Network.PublicKey))

	resp := &pb.AddMachineResponse{Machine: m}
	return resp, nil
}

func (c *Cluster) network(ctx context.Context) (netip.Prefix, error) {
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

// UpdateMachine updates machine configuration in the cluster.
func (c *Cluster) UpdateMachine(ctx context.Context, req *pb.UpdateMachineRequest) (*pb.UpdateMachineResponse, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	if req.MachineId == "" {
		return nil, status.Error(codes.InvalidArgument, "machine_id not set")
	}

	// Get the current machine info
	currentMachine, err := c.store.GetMachine(ctx, req.MachineId)
	if err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			return nil, status.Errorf(codes.NotFound, "machine not found: %s", req.MachineId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get machine: %v", err)
	}

	// Create a copy of the current machine for updating
	updatedMachine := &pb.MachineInfo{
		Id:       currentMachine.Id,
		Name:     currentMachine.Name,
		Network:  currentMachine.Network,
		PublicIp: currentMachine.PublicIp,
	}

	// Apply updates from the request
	if req.Name != nil {
		// Check for empty name
		if *req.Name == "" {
			return nil, status.Error(codes.InvalidArgument, "machine name cannot be empty")
		}
		// Check for duplicate names (excluding the current machine)
		if *req.Name != currentMachine.Name {
			machines, err := c.store.ListMachines(ctx)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "list machines: %v", err)
			}
			for _, m := range machines {
				if m.Id != req.MachineId && m.Name == *req.Name {
					return nil, status.Errorf(codes.AlreadyExists, "machine with name %q already exists", *req.Name)
				}
			}
		}
		updatedMachine.Name = *req.Name
	}
	if req.PublicIp != nil {
		// Check if this is an empty IP (used to signal removal)
		if len(req.PublicIp.Ip) == 0 {
			// User wants to remove public IP
			updatedMachine.PublicIp = nil
		} else {
			// Validate and set the new IP
			ip, err := req.PublicIp.ToAddr()
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid public IP: %v", err)
			}
			if !ip.IsValid() {
				return nil, status.Error(codes.InvalidArgument, "invalid public IP")
			}
			updatedMachine.PublicIp = req.PublicIp
		}
	}
	if req.Endpoints != nil {
		updatedMachine.Network.Endpoints = req.Endpoints
	}

	// Update the machine in the store
	if err = c.store.UpdateMachine(ctx, updatedMachine); err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			return nil, status.Errorf(codes.NotFound, "machine not found: %s", req.MachineId)
		}
		return nil, status.Errorf(codes.Internal, "update machine: %v", err)
	}

	slog.Info("Machine configuration updated in the cluster.",
		"id", updatedMachine.Id, "name", updatedMachine.Name)

	resp := &pb.UpdateMachineResponse{Machine: updatedMachine}
	return resp, nil
}

// ListMachines lists all machines in the cluster including their membership states.
func (c *Cluster) ListMachines(ctx context.Context, _ *emptypb.Empty) (*pb.ListMachinesResponse, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	machines, err := c.store.ListMachines(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	states, err := c.corroAdmin.ClusterMembershipStates(true)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster membership states: %v", err)
	}

	members := make([]*pb.MachineMember, len(machines))
	for i, m := range machines {
		// If the machine is not in the cluster membership states or its state is not ALIVE or SUSPECT, it is DOWN.
		// The exception is the current machine which is always UP as it is serving this request.
		state := pb.MachineMember_DOWN
		addr, _ := m.Network.ManagementIp.ToAddr()
		for _, s := range states {
			if s.Addr.Addr().Compare(addr) == 0 {
				switch s.State {
				case corrosion.MembershipStateAlive:
					state = pb.MachineMember_UP
				case corrosion.MembershipStateSuspect:
					state = pb.MachineMember_SUSPECT
				}
				break
			}
		}
		// If the machine is the current machine, it is UP.
		if m.Id == c.machineID {
			state = pb.MachineMember_UP
		}
		members[i] = &pb.MachineMember{
			Machine: m,
			State:   state,
		}
	}

	return &pb.ListMachinesResponse{Machines: members}, nil
}

// RemoveMachine removes a machine from the cluster.
func (c *Cluster) RemoveMachine(ctx context.Context, req *pb.RemoveMachineRequest) (*emptypb.Empty, error) {
	if err := c.checkReady(); err != nil {
		return nil, err
	}

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "machine ID not set")
	}

	if err := c.store.DeleteMachine(ctx, req.Id); err != nil {
		if errors.Is(err, store.ErrMachineNotFound) {
			return nil, status.Errorf(codes.NotFound, "machine not found: %s", req.Id)
		}
		return nil, status.Errorf(codes.Internal, "delete machine from store: %v", err)
	}
	slog.Info("Machine removed from the cluster.", "id", req.Id)

	return &emptypb.Empty{}, nil
}
