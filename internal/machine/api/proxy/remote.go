package proxy

import (
	"context"
	"fmt"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"net"
	"net/netip"
	"sync"
	"time"
	"uncloud/internal/machine/api/pb"
)

// RemoteBackend is a proxy.Backend implementation that proxies to a remote gRPC server, injecting machine metadata
// into the response.
//
// Based on the Talos apid implementation:
// https://github.com/siderolabs/talos/blob/59a78da42cdea8fbccc35d0851f9b0eef928261b/internal/app/apid/pkg/backend/apid.go
type RemoteBackend struct {
	target string

	mu   sync.RWMutex
	conn *grpc.ClientConn
}

var _ proxy.Backend = (*RemoteBackend)(nil)

// NewRemoteBackend creates a new instance of RemoteBackend for the given target which must have the format [IPv6]:port.
func NewRemoteBackend(target string) (*RemoteBackend, error) {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("target must have the format [IPv6]:port: %s", target)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Is6() {
		return nil, fmt.Errorf("target host must be a valid IPv6 address: %s", host)
	}

	return &RemoteBackend{target: target}, nil
}

func (b *RemoteBackend) String() string {
	return b.target
}

// GetConnection returns a gRPC connection to the remote server.
func (b *RemoteBackend) GetConnection(ctx context.Context, _ string) (context.Context, *grpc.ClientConn, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if authority := md[":authority"]; len(authority) > 0 {
		md.Set("proxy-authority", authority...)
	} else {
		md.Set("proxy-authority", "unknown")
	}
	delete(md, ":authority")
	delete(md, "machines")

	outCtx := metadata.NewOutgoingContext(ctx, md)

	b.mu.RLock()
	if b.conn != nil {
		defer b.mu.RUnlock()
		return outCtx, b.conn, nil
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Override the max delay to avoid excessive backoff when the another node is unavailable, e.g. rebooted
	// or the WireGuard connection is temporary down.
	//
	// Default max delay is 2 minutes, which is too long for our use case.
	backoffConfig := backoff.DefaultConfig
	// The maximum wait time between attempts.
	backoffConfig.MaxDelay = 15 * time.Second

	var err error
	b.conn, err = grpc.NewClient(
		b.target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoffConfig,
			// Not published as a constant in gRPC library.
			// See: https://github.com/grpc/grpc-go/blob/d5dee5fdbdeb52f6ea10b37b2cc7ce37814642d7/clientconn.go#L55-L56
			// Each connection attempt can take up to MinConnectTimeout.
			MinConnectTimeout: 20 * time.Second,
		}),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodecV2(proxy.Codec()),
		),
	)

	return outCtx, b.conn, err
}

// AppendInfo is called to enhance response from the backend with additional data.
//
// AppendInfo enhances upstream response with machine metadata (target).
//
// This method depends on grpc protobuf response structure, each response should
// look like:
//
//	message SomeResponse {
//	  repeated SomeReply messages = 1; // please note field ID == 1
//	}
//
//	message SomeReply {
//	  common.Metadata metadata = 1;
//	  <other fields go here ...>
//	}
//
// As 'SomeReply' is repeated in 'SomeResponse', if we concatenate protobuf representation
// of several 'SomeResponse' messages, we still get valid 'SomeResponse' representation but with more
// entries (feature of protobuf binary representation).
//
// If we look at binary representation of any unary 'SomeResponse' message, it will always contain one
// protobuf field with field ID 1 (see above) and type 2 (embedded message SomeReply is encoded
// as string with length). So if we want to add fields to 'SomeReply', we can simply read field
// header, adjust length for new 'SomeReply' representation, and prepend new field header.
//
// At the same time, we can add 'common.Metadata' structure to 'SomeReply' by simply
// appending or prepending 'common.Metadata' as a single field. This requires 'metadata'
// field to be not defined in original response. (This is due to the fact that protobuf message
// representation is concatenation of each field representation).
//
// To build only single field (Metadata) we use helper message which contains exactly this
// field with same field ID as in every other 'SomeReply':
//
//	message Empty {
//	  common.Metadata metadata = 1;
//	}
//
// As streaming replies are not wrapped into 'SomeResponse' with 'repeated', handling is simpler: we just
// need to append Empty with details.
//
// So AppendInfo does the following: validates that response contains field ID 1 encoded as string,
// cuts field header, rest is representation of some reply. Marshal 'Empty' as protobuf,
// which builds 'common.Metadata' field, append it to original response message, build new header
// for new length of some response, and add back new field header.
func (b *RemoteBackend) AppendInfo(streaming bool, resp []byte) ([]byte, error) {
	payload, err := proto.Marshal(&pb.Empty{
		Metadata: &pb.Metadata{
			Machine: b.target,
		},
	})

	if streaming {
		return append(resp, payload...), err
	}

	const (
		metadataField = 1 // field number in proto definition for repeated response
		metadataType  = 2 // "string" for embedded messages
	)

	// decode protobuf embedded header

	typ, n1 := protowire.ConsumeVarint(resp)
	if n1 < 0 {
		return nil, protowire.ParseError(n1)
	}

	_, n2 := protowire.ConsumeVarint(resp[n1:]) // length
	if n2 < 0 {
		return nil, protowire.ParseError(n2)
	}

	if typ != (metadataField<<3)|metadataType {
		return nil, fmt.Errorf("unexpected message format: %d", typ)
	}

	if n1+n2 > len(resp) {
		return nil, fmt.Errorf("unexpected message size: %d", len(resp))
	}

	// cut off embedded message header
	resp = resp[n1+n2:]
	// build new embedded message header
	prefix := protowire.AppendVarint(
		protowire.AppendVarint(nil, (metadataField<<3)|metadataType),
		uint64(len(resp)+len(payload)),
	)
	resp = append(prefix, resp...)

	return append(resp, payload...), err
}

// BuildError converts upstream error into message from upstream, so that multiple
// successful and failure responses might be returned.
//
// This simply relies on the fact that any response contains 'Empty' message.
// So if 'Empty' is unmarshalled into any other reply message, all the fields
// are undefined but 'Metadata':
//
//	message Empty {
//	  common.Metadata metadata = 1;
//	}
//
//	message EmptyResponse {
//	  repeated Empty messages = 1;
//	}
//
// Streaming responses are not wrapped into Empty, so we simply marshall EmptyResponse
// message.
func (b *RemoteBackend) BuildError(streaming bool, err error) ([]byte, error) {
	var resp proto.Message = &pb.Empty{
		Metadata: &pb.Metadata{
			Machine: b.target,
			Error:   err.Error(),
			Status:  status.Convert(err).Proto(),
		},
	}

	if !streaming {
		resp = &pb.EmptyResponse{
			Messages: []*pb.Empty{
				resp.(*pb.Empty),
			},
		}
	}

	return proto.Marshal(resp)
}

// Close closes the upstream gRPC connection.
func (b *RemoteBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
	}
}
