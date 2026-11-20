package tunnel

import (
	"context"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/psviderski/uncloud/api/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	handshakeTimeout = 10 * time.Second
	// kdfInfo separates the keys derived for tunnel proofs from any other use of the WireGuard key.
	kdfInfo = "uncloud-hub-tunnel-v1"
	// hubAddr is the address of the Hub end of a tunnel connection.
	hubAddr = Addr("hub")
)

// ErrUnknownMachine is returned by a KeyResolver when Hub doesn't know the machine.
var ErrUnknownMachine = errors.New("unknown machine")

// errUnauthenticated is the same for an unknown machine and an invalid proof so that the response doesn't reveal
// which machines Hub knows.
var errUnauthenticated = status.Error(codes.Unauthenticated, "Hub doesn't know the machine or its proof is invalid")

// Credentials authenticate a machine to Hub.
type Credentials struct {
	ClusterID string
	MachineID string
	// PrivateKey is the WireGuard private key of the machine.
	PrivateKey []byte
}

// Machine identifies the machine at the other end of a tunnel accepted by Hub.
type Machine struct {
	ClusterID string
	MachineID string
}

// KeyResolver returns the WireGuard public key that Hub knows for a machine. It returns an error that wraps
// ErrUnknownMachine if Hub doesn't know the machine.
type KeyResolver func(ctx context.Context, clusterID, machineID string) ([]byte, error)

// Open opens a tunnel to Hub and authenticates the machine with its WireGuard key. The tunnel stays open until ctx is
// cancelled, the returned connection is closed, or Hub ends it. If Hub rejects the machine, Open returns the gRPC
// status error Unauthenticated.
func Open(ctx context.Context, client pb.HubClient, creds Credentials) (*Conn, error) {
	priv, err := ecdh.X25519().NewPrivateKey(creds.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid WireGuard private key: %w", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := client.Tunnel(streamCtx)
	if err != nil {
		cancel()
		return nil, err
	}

	// Cancel the stream if Hub doesn't complete the handshake in time.
	timer := time.AfterFunc(handshakeTimeout, cancel)
	err = machineHandshake(stream, priv, creds)
	// The timer has cancelled the stream if it can't be stopped, even if the handshake has just completed.
	if !timer.Stop() {
		cancel()
		return nil, status.Error(codes.DeadlineExceeded, "tunnel handshake timed out")
	}
	if err != nil {
		cancel()
		return nil, err
	}

	return newConn(stream, cancel, Addr(creds.MachineID), hubAddr), nil
}

func machineHandshake(stream pb.Hub_TunnelClient, priv *ecdh.PrivateKey, creds Credentials) error {
	hello := &pb.TunnelMessage{Message: &pb.TunnelMessage_Hello{Hello: &pb.TunnelHello{
		ClusterId: creds.ClusterID,
		MachineId: creds.MachineID,
	}}}
	if err := stream.Send(hello); err != nil {
		return machineSendError(stream, err)
	}

	msg, err := machineRecv(stream)
	if err != nil {
		return err
	}
	challenge := msg.GetChallenge()
	if challenge == nil {
		return errors.New("unexpected message from Hub: expected challenge")
	}
	challengeKey, err := ecdh.X25519().NewPublicKey(challenge.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid challenge key from Hub: %w", err)
	}
	shared, err := priv.ECDH(challengeKey)
	if err != nil {
		return fmt.Errorf("invalid challenge key from Hub: %w", err)
	}
	hmac, err := computeHMAC(shared, creds.ClusterID, creds.MachineID, priv.PublicKey().Bytes(), challenge.PublicKey)
	if err != nil {
		return err
	}

	proof := &pb.TunnelMessage{Message: &pb.TunnelMessage_Proof{Proof: &pb.TunnelProof{Hmac: hmac}}}
	if err = stream.Send(proof); err != nil {
		return machineSendError(stream, err)
	}

	msg, err = machineRecv(stream)
	if err != nil {
		return err
	}
	if msg.GetAccepted() == nil {
		return errors.New("unexpected message from Hub: expected accepted")
	}

	return nil
}

// machineRecv receives a handshake message and reports a stream that Hub ended without an error as an error.
func machineRecv(stream pb.Hub_TunnelClient) (*pb.TunnelMessage, error) {
	msg, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("Hub ended the tunnel during the handshake")
	}
	return msg, err
}

// machineSendError returns the status that Hub ended the stream with if sending failed because the stream ended.
func machineSendError(stream pb.Hub_TunnelClient, err error) error {
	if !errors.Is(err, io.EOF) {
		return err
	}
	_, err = machineRecv(stream)
	return err
}

// Accept authenticates the machine that opens a tunnel stream and returns the tunnel connection for a Hub gRPC
// client. Hub should call it in its Tunnel handler, which must return once the connection's Done channel is closed.
// The returned error is a gRPC status error that the handler can return as is.
func Accept(stream pb.Hub_TunnelServer, resolve KeyResolver) (*Conn, Machine, error) {
	ctx, cancel := context.WithTimeout(stream.Context(), handshakeTimeout)
	defer cancel()

	type result struct {
		machine Machine
		err     error
	}
	// Receiving from a server stream can't be cancelled, so run the handshake in a goroutine to enforce the timeout.
	// The goroutine exits when the handler returns and the stream ends.
	resultCh := make(chan result, 1)
	go func() {
		m, err := hubHandshake(ctx, stream, resolve)
		resultCh <- result{machine: m, err: err}
	}()

	select {
	case r := <-resultCh:
		if r.err != nil {
			return nil, Machine{}, r.err
		}
		return newConn(stream, nil, hubAddr, Addr(r.machine.MachineID)), r.machine, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, Machine{}, status.Error(codes.DeadlineExceeded, "tunnel handshake timed out")
		}
		return nil, Machine{}, status.FromContextError(ctx.Err()).Err()
	}
}

func hubHandshake(ctx context.Context, stream pb.Hub_TunnelServer, resolve KeyResolver) (Machine, error) {
	msg, err := stream.Recv()
	if err != nil {
		return Machine{}, err
	}
	hello := msg.GetHello()
	if hello == nil || hello.ClusterId == "" || hello.MachineId == "" {
		return Machine{}, status.Error(codes.InvalidArgument, "expected hello with cluster and machine IDs")
	}

	// Continue the handshake for an unknown machine and reject it only after the proof, like an invalid proof.
	var machineKey *ecdh.PublicKey
	keyBytes, err := resolve(ctx, hello.ClusterId, hello.MachineId)
	if err != nil {
		if !errors.Is(err, ErrUnknownMachine) {
			return Machine{}, status.Errorf(codes.Unavailable, "resolve machine key: %v", err)
		}
	} else if machineKey, err = ecdh.X25519().NewPublicKey(keyBytes); err != nil {
		machineKey = nil
	}

	challengeKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Machine{}, status.Errorf(codes.Internal, "generate challenge key: %v", err)
	}
	challenge := &pb.TunnelMessage{Message: &pb.TunnelMessage_Challenge{Challenge: &pb.TunnelChallenge{
		PublicKey: challengeKey.PublicKey().Bytes(),
	}}}
	if err = stream.Send(challenge); err != nil {
		return Machine{}, err
	}

	msg, err = stream.Recv()
	if err != nil {
		return Machine{}, err
	}
	proof := msg.GetProof()
	if proof == nil {
		return Machine{}, status.Error(codes.InvalidArgument, "expected proof")
	}
	if machineKey == nil {
		return Machine{}, errUnauthenticated
	}
	// ECDH fails for a low-order machine key.
	shared, err := challengeKey.ECDH(machineKey)
	if err != nil {
		return Machine{}, errUnauthenticated
	}
	expected, err := computeHMAC(shared, hello.ClusterId, hello.MachineId,
		machineKey.Bytes(), challengeKey.PublicKey().Bytes())
	if err != nil {
		return Machine{}, status.Errorf(codes.Internal, "compute proof: %v", err)
	}
	if !hmac.Equal(expected, proof.Hmac) {
		return Machine{}, errUnauthenticated
	}

	accepted := &pb.TunnelMessage{Message: &pb.TunnelMessage_Accepted{Accepted: &pb.TunnelAccepted{}}}
	if err = stream.Send(accepted); err != nil {
		return Machine{}, err
	}

	return Machine{ClusterID: hello.ClusterId, MachineID: hello.MachineId}, nil
}

// computeHMAC returns the proof for the handshake transcript keyed with a key derived from the X25519 shared secret.
// Each transcript field is prefixed with its length so that different fields can't produce the same input.
func computeHMAC(shared []byte, clusterID, machineID string, machineKey, challengeKey []byte) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, shared, nil, kdfInfo, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("derive proof key: %w", err)
	}

	mac := hmac.New(sha256.New, key)
	for _, field := range [][]byte{[]byte(clusterID), []byte(machineID), machineKey, challengeKey} {
		mac.Write(binary.BigEndian.AppendUint32(nil, uint32(len(field))))
		mac.Write(field)
	}
	return mac.Sum(nil), nil
}
