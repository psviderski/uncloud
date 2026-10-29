package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestProxySingleMachineContext(t *testing.T) {
	original := metadata.Pairs(
		"authorization", "token",
		"machine", "old-machine",
		"machines", "old-machine-a",
		"machines", "old-machine-b",
	)
	ctx := metadata.NewOutgoingContext(context.Background(), original)

	proxyCtx := ProxySingleMachineContext(ctx, "new-machine")

	md, ok := metadata.FromOutgoingContext(proxyCtx)
	require.True(t, ok)
	require.Equal(t, metadata.Pairs(
		"authorization", "token",
		"machine", "new-machine",
	), md)
	require.Equal(t, metadata.Pairs(
		"authorization", "token",
		"machine", "old-machine",
		"machines", "old-machine-a",
		"machines", "old-machine-b",
	), original)
}

func TestProxyMachinesContext(t *testing.T) {
	tests := []struct {
		name     string
		machines []string
		want     []string
	}{
		{
			name:     "specified machines",
			machines: []string{"machine-a", "machine-b"},
			want:     []string{"machine-a", "machine-b"},
		},
		{
			name: "all machines",
			want: []string{"*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := metadata.Pairs(
				"authorization", "token",
				"machine", "old-machine",
				"machines", "old-machine-a",
				"machines", "old-machine-b",
			)
			ctx := metadata.NewOutgoingContext(context.Background(), original)

			proxyCtx := ProxyMachinesContext(ctx, tt.machines)

			md, ok := metadata.FromOutgoingContext(proxyCtx)
			require.True(t, ok)
			require.Equal(t, metadata.MD{
				"authorization": {"token"},
				"machines":      tt.want,
			}, md)
			require.Equal(t, metadata.Pairs(
				"authorization", "token",
				"machine", "old-machine",
				"machines", "old-machine-a",
				"machines", "old-machine-b",
			), original)
		})
	}
}
