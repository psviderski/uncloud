package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// MachineToken contains the parsed enrollment token from a machine.
type MachineToken struct {
	PublicKey []byte           `json:"PublicKey"`
	PublicIP  netip.Addr       `json:"PublicIP"`
	Endpoints []netip.AddrPort `json:"Endpoints"`
}

const machineTokenPrefix = "mtkn:"

// ParseMachineToken decodes a machine enrollment token string.
func ParseMachineToken(s string) (MachineToken, error) {
	if !strings.HasPrefix(s, machineTokenPrefix) {
		return MachineToken{}, fmt.Errorf("invalid token prefix: expected %q", machineTokenPrefix)
	}
	decoded, err := base64.StdEncoding.DecodeString(s[len(machineTokenPrefix):])
	if err != nil {
		return MachineToken{}, fmt.Errorf("decode token: %w", err)
	}
	var token MachineToken
	if err := json.Unmarshal(decoded, &token); err != nil {
		return MachineToken{}, fmt.Errorf("unmarshal token: %w", err)
	}
	return token, nil
}

// ToNetworkConfig converts the token to a NetworkConfig for use with AddMachine.
func (t MachineToken) ToNetworkConfig() *pb.NetworkConfig {
	return NewNetworkConfig(t.Endpoints, t.PublicKey)
}

// NewNetworkConfig creates a NetworkConfig from Go-native types.
func NewNetworkConfig(endpoints []netip.AddrPort, publicKey []byte) *pb.NetworkConfig {
	eps := make([]*pb.IPPort, len(endpoints))
	for i, ep := range endpoints {
		eps[i] = pb.NewIPPort(ep)
	}
	return &pb.NetworkConfig{
		Endpoints: eps,
		PublicKey: publicKey,
	}
}

// AdoptMachineOptions configures the AdoptMachine operation.
type AdoptMachineOptions struct {
	// Name is the name to assign to the machine. If empty, a random name is generated.
	Name string
	// PublicIP configures the machine's public IP for ingress.
	// nil = no ingress, zero value = use auto-detected IP from token, valid IP = use that IP.
	PublicIP *netip.Addr
	// WaitReady waits for the machine to be ready after joining. Defaults to true.
	WaitReady bool
	// WaitTimeout is how long to wait for the machine to be ready. Defaults to 5 minutes.
	WaitTimeout time.Duration
}

// AdoptMachine adds a new machine to this cluster. The newMachineClient should be
// connected to a machine with the daemon running but not yet joined to any cluster.
//
// This performs the full add flow:
//  1. Check prerequisites on new machine
//  2. Get enrollment token (WireGuard pubkey + endpoints)
//  3. Register machine in cluster (allocates subnet, generates ID)
//  4. Get peer configuration from existing cluster machines
//  5. Configure new machine to join with peer info
//  6. Optionally wait for the machine to be ready
func (cli *Client) AdoptMachine(
	ctx context.Context,
	newMachineClient *Client,
	opts AdoptMachineOptions,
) (*pb.MachineInfo, error) {
	if opts.WaitTimeout == 0 {
		opts.WaitTimeout = 5 * time.Minute
	}

	// 1. Check prerequisites
	satisfied, errMsg, err := newMachineClient.CheckPrerequisites(ctx)
	if err != nil {
		return nil, fmt.Errorf("check prerequisites: %w", err)
	}
	if !satisfied {
		return nil, fmt.Errorf("machine prerequisites not satisfied: %s", errMsg)
	}

	// 2. Get enrollment token
	tokenStr, err := newMachineClient.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("get machine token: %w", err)
	}
	token, err := ParseMachineToken(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("parse machine token: %w", err)
	}

	// 3. Determine public IP
	var pubIP netip.Addr
	if opts.PublicIP != nil {
		if opts.PublicIP.IsValid() {
			pubIP = *opts.PublicIP
		} else {
			pubIP = token.PublicIP
		}
	}

	// 4. Register machine in cluster
	machineInfo, err := cli.AddMachine(ctx, opts.Name, token.ToNetworkConfig(), pubIP)
	if err != nil {
		return nil, fmt.Errorf("add machine to cluster: %w", err)
	}

	// 5. Get store DB version for sync
	var storeDBVersion int64
	resp, err := cli.MachineClient.InspectMachine(ctx, &emptypb.Empty{})
	if err == nil && len(resp.Machines) > 0 {
		storeDBVersion = resp.Machines[0].StoreDbVersion
	}

	// 6. Get other machines for peer config
	machines, err := cli.ListMachines(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	others := make([]*pb.MachineInfo, 0, len(machines)-1)
	for _, m := range machines {
		if m.Machine.Id != machineInfo.Id {
			others = append(others, m.Machine)
		}
	}

	// 7. Join cluster
	if err := newMachineClient.JoinCluster(ctx, machineInfo, others, storeDBVersion); err != nil {
		return nil, fmt.Errorf("join cluster: %w", err)
	}

	// 8. Wait for ready
	if opts.WaitReady {
		if err := newMachineClient.WaitClusterReady(ctx, opts.WaitTimeout); err != nil {
			return nil, fmt.Errorf("wait for machine ready: %w", err)
		}
	}

	return machineInfo, nil
}

// Token returns the machine enrollment token.
func (cli *Client) Token(ctx context.Context) (string, error) {
	resp, err := cli.MachineClient.Token(ctx, &emptypb.Empty{})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// CheckPrerequisites verifies the machine meets system requirements.
func (cli *Client) CheckPrerequisites(ctx context.Context) (satisfied bool, errMsg string, err error) {
	resp, err := cli.MachineClient.CheckPrerequisites(ctx, &emptypb.Empty{})
	if err != nil {
		return false, "", err
	}
	return resp.Satisfied, resp.Error, nil
}

// AddMachine registers a new machine to the cluster.
func (cli *Client) AddMachine(
	ctx context.Context, name string, network *pb.NetworkConfig, publicIP netip.Addr,
) (*pb.MachineInfo, error) {
	req := &pb.AddMachineRequest{
		Name:    name,
		Network: network,
	}
	if publicIP.IsValid() {
		req.PublicIp = pb.NewIP(publicIP)
	}
	resp, err := cli.ClusterClient.AddMachine(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Machine, nil
}

// JoinCluster configures the connected machine to join an existing cluster.
func (cli *Client) JoinCluster(
	ctx context.Context, machine *pb.MachineInfo, others []*pb.MachineInfo, minDBVersion int64,
) error {
	req := &pb.JoinClusterRequest{
		Machine:           machine,
		OtherMachines:     others,
		MinStoreDbVersion: minDBVersion,
	}
	_, err := cli.MachineClient.JoinCluster(ctx, req)
	return err
}
