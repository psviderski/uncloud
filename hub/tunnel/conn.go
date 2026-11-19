package tunnel

import (
	"context"
	"errors"

	"github.com/psviderski/uncloud/api/pb"
	grpcnetconn "github.com/unlabs-dev/go-grpc-net-conn"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	// Network is the network name of tunnel connection addresses.
	Network = "hub-tunnel"
	// maxDataSize is the maximum number of bytes in one data message. gRPC writes HTTP/2 frames through a 32 KiB
	// buffer so most writes fit in one message.
	maxDataSize = 32 * 1024
)

var errUnexpectedMessage = errors.New("unexpected tunnel message: expected data")

// Addr is the address of an end of a tunnel connection.
type Addr string

func (a Addr) Network() string {
	return Network
}

func (a Addr) String() string {
	return string(a)
}

// Conn is a net.Conn over a tunnel stream after a successful handshake. Done and Err report when and why it ended.
// On the machine side, Close cancels the tunnel stream. On the Hub side, Close closes Done so the Tunnel handler can
// return and end the stream.
type Conn = grpcnetconn.Conn

func newConn(stream grpc.Stream, cancel context.CancelFunc, local, remote Addr) *Conn {
	return &Conn{
		Stream:   stream,
		Request:  &pb.TunnelMessage{},
		Response: &pb.TunnelMessage{},
		Encode:   grpcnetconn.ChunkedEncoder(encodeData, maxDataSize),
		Decode:   decodeData,
		Cancel:   cancel,
		Local:    local,
		Remote:   remote,
	}
}

func encodeData(msg proto.Message, p []byte) (int, error) {
	msg.(*pb.TunnelMessage).Message = &pb.TunnelMessage_Data{Data: p}
	return len(p), nil
}

func decodeData(msg proto.Message, offset int, p []byte) ([]byte, error) {
	data, ok := msg.(*pb.TunnelMessage).Message.(*pb.TunnelMessage_Data)
	if !ok {
		return nil, errUnexpectedMessage
	}

	copy(p, data.Data[offset:])
	return data.Data, nil
}
