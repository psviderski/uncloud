package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/siderolabs/grpc-proxy/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockMapper struct {
	targets []MachineTarget
	err     error
}

func (m *mockMapper) MapMachines(_ context.Context, _ []string) ([]MachineTarget, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.targets, nil
}

func TestDirector_Director(t *testing.T) {
	d := NewDirector("/tmp/test.sock", 8080, nil)
	t.Cleanup(d.Close)

	// Use a valid IPv6 address for remote targets.
	remoteTarget := MachineTarget{ID: "id-2", Name: "machine-b", Addr: "fd00::2"}
	localTarget := MachineTarget{ID: "id-1", Name: "machine-a", Addr: "fd00::1"}

	t.Run("no metadata routes to local", func(t *testing.T) {
		ctx := context.Background()
		mode, backends, err := d.Director(ctx, "/Test/Method")

		require.NoError(t, err)
		assert.Equal(t, proxy.One2One, mode)
		assert.Len(t, backends, 1)
		assert.IsType(t, (*LocalBackend)(nil), backends[0])
	})

	t.Run("proxy-authority routes to local", func(t *testing.T) {
		md := metadata.New(map[string]string{"proxy-authority": "test"})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		mode, backends, err := d.Director(ctx, "/Test/Method")

		require.NoError(t, err)
		assert.Equal(t, proxy.One2One, mode)
		assert.Len(t, backends, 1)
		assert.IsType(t, (*LocalBackend)(nil), backends[0])
	})

	t.Run("machine singular local", func(t *testing.T) {
		d.localAddress.Store(localTarget.Addr)
		d.mapper = &mockMapper{targets: []MachineTarget{localTarget}}

		md := metadata.New(map[string]string{"machine": localTarget.Name})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		mode, backends, err := d.Director(ctx, "/Test/Method")

		require.NoError(t, err)
		assert.Equal(t, proxy.One2One, mode)
		assert.Len(t, backends, 1)
		assert.IsType(t, (*LocalBackend)(nil), backends[0])
	})

	t.Run("machine singular remote", func(t *testing.T) {
		d.localAddress.Store(localTarget.Addr)
		d.mapper = &mockMapper{targets: []MachineTarget{remoteTarget}}

		md := metadata.New(map[string]string{"machine": remoteTarget.Name})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		mode, backends, err := d.Director(ctx, "/Test/Method")

		require.NoError(t, err)
		assert.Equal(t, proxy.One2One, mode)
		assert.Len(t, backends, 1)
		assert.IsType(t, (*RemoteBackend)(nil), backends[0])
	})

	t.Run("machine not found", func(t *testing.T) {
		d.mapper = &mockMapper{err: &MachinesNotFoundError{NotFound: []string{"missing"}}}

		md := metadata.New(map[string]string{"machine": "missing"})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "machine not found: missing")
	})

	t.Run("machines plural single local", func(t *testing.T) {
		d.localAddress.Store(localTarget.Addr)
		d.mapper = &mockMapper{targets: []MachineTarget{localTarget}}

		md := metadata.Pairs("machines", localTarget.Name)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		mode, backends, err := d.Director(ctx, "/Test/Method")

		require.NoError(t, err)
		assert.Equal(t, proxy.One2Many, mode)
		assert.Len(t, backends, 1)

		mb := backends[0].(*MetadataBackend)
		assert.Equal(t, localTarget.ID, mb.MachineID)
		assert.Equal(t, localTarget.Name, mb.MachineName)
		assert.Equal(t, localTarget.Addr, mb.MachineAddr)
		assert.IsType(t, (*LocalBackend)(nil), mb.Backend)
	})

	t.Run("machines plural multiple", func(t *testing.T) {
		d.localAddress.Store(localTarget.Addr)
		d.mapper = &mockMapper{targets: []MachineTarget{localTarget, remoteTarget}}

		md := metadata.Pairs("machines", localTarget.Name, "machines", remoteTarget.Name)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		mode, backends, err := d.Director(ctx, "/Test/Method")

		require.NoError(t, err)
		assert.Equal(t, proxy.One2Many, mode)
		assert.Len(t, backends, 2)

		// First backend should be local.
		mb0 := backends[0].(*MetadataBackend)
		assert.Equal(t, localTarget.ID, mb0.MachineID)
		assert.IsType(t, (*LocalBackend)(nil), mb0.Backend)

		// Second backend should be remote.
		mb1 := backends[1].(*MetadataBackend)
		assert.Equal(t, remoteTarget.ID, mb1.MachineID)
		assert.IsType(t, (*RemoteBackend)(nil), mb1.Backend)
	})

	t.Run("machines empty string", func(t *testing.T) {
		d.mapper = &mockMapper{err: &MachinesNotFoundError{NotFound: []string{""}}}

		md := metadata.Pairs("machines", "")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "machine not found")
	})

	t.Run("machine empty slice", func(t *testing.T) {
		md := metadata.MD{"machine": []string{}}
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "no machines specified")
	})

	t.Run("machines empty slice", func(t *testing.T) {
		md := metadata.MD{"machines": []string{}}
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "no machines specified")
	})

	t.Run("both machine and machines set", func(t *testing.T) {
		md := metadata.Pairs("machine", "m1", "machines", "m1", "machines", "m2")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "both 'machine' and 'machines' proxy metadata are set")
	})

	t.Run("machines not found", func(t *testing.T) {
		d.mapper = &mockMapper{err: &MachinesNotFoundError{NotFound: []string{"missing"}}}

		md := metadata.Pairs("machines", "missing")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("machines mapper generic error", func(t *testing.T) {
		d.mapper = &mockMapper{err: errors.New("boom")}

		md := metadata.Pairs("machines", "any")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, _, err := d.Director(ctx, "/Test/Method")

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
	})
}

func TestMapErrorToStatus(t *testing.T) {
	t.Run("machines not found", func(t *testing.T) {
		err := mapErrorToStatus(&MachinesNotFoundError{NotFound: []string{"a", "b"}})
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("already grpc status", func(t *testing.T) {
		original := status.Error(codes.DeadlineExceeded, "timeout")
		err := mapErrorToStatus(original)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.DeadlineExceeded, st.Code())
	})

	t.Run("generic error", func(t *testing.T) {
		err := mapErrorToStatus(errors.New("something broke"))
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Contains(t, st.Message(), "something broke")
	})
}
