package tunnel

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync/atomic"

	"github.com/psviderski/uncloud/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NewAPIConn creates a gRPC client connection to the Uncloud API that the machine serves through the tunnel connection.
// The client connection connects right away so that the machine can start sending keepalive through the tunnel.
// It closes when the tunnel connection ends and can't reconnect, so a new one needs to be created for each tunnel.
// Options from opts are applied after the default ones.
func NewAPIConn(conn *Conn, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	var dialed atomic.Bool
	dialer := func(context.Context, string) (net.Conn, error) {
		if dialed.Swap(true) {
			return nil, errors.New("tunnel connection is already used")
		}
		return conn, nil
	}

	opts = slices.Concat([]grpc.DialOption{
		grpc.WithContextDialer(dialer),
		// The tunnel stream is already encrypted with TLS.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Disable idleness because closing the idle transport would close the tunnel connection.
		grpc.WithIdleTimeout(0),
	}, opts)
	cc, err := client.NewConn("passthrough:///"+Network, opts...)
	if err != nil {
		return nil, err
	}

	// The client connection would otherwise connect only on the first call. The machine's keepalive only runs on
	// an established connection, so an unused tunnel would carry no data and proxies would close it as idle.
	cc.Connect()

	go func() {
		<-conn.Done()
		cc.Close()
	}()

	return cc, nil
}
