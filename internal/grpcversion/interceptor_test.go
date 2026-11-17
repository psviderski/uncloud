package grpcversion

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestParseVersionOrZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid semver",
			input:    "1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "valid semver with prerelease",
			input:    "0.19.0-nightly-abc1234",
			expected: "0.19.0-nightly-abc1234",
		},
		{
			name:     "go module pseudo-version",
			input:    "0.22.1-0.20261117120000-abcdef123456",
			expected: "0.22.1-0.20261117120000-abcdef123456",
		},
		{
			name:     "dev version",
			input:    "999.0.0-dev",
			expected: "999.0.0-dev",
		},
		{
			name:     "invalid ldflag-injected string falls back to zero",
			input:    "nightly-SNAPSHOT-abc1234",
			expected: "0.0.0",
		},
		{
			name:     "empty string falls back to zero",
			input:    "",
			expected: "0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersionOrZero(tt.input)
			assert.Equal(t, tt.expected, got.String())
		})
	}
}

func TestExtractVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		md       metadata.MD
		key      string
		expected string
	}{
		{
			name:     "nil metadata",
			md:       nil,
			key:      MetadataKeyClientVersion,
			expected: "0.0.0",
		},
		{
			name:     "missing key",
			md:       metadata.MD{},
			key:      MetadataKeyClientVersion,
			expected: "0.0.0",
		},
		{
			name:     "empty value",
			md:       metadata.Pairs(MetadataKeyClientVersion, ""),
			key:      MetadataKeyClientVersion,
			expected: "0.0.0",
		},
		{
			name:     "invalid version",
			md:       metadata.Pairs(MetadataKeyClientVersion, "not-a-version"),
			key:      MetadataKeyClientVersion,
			expected: "0.0.0",
		},
		{
			name:     "valid version",
			md:       metadata.Pairs(MetadataKeyClientVersion, "1.2.3"),
			key:      MetadataKeyClientVersion,
			expected: "1.2.3",
		},
		{
			name:     "version with prerelease",
			md:       metadata.Pairs(MetadataKeyClientVersion, "0.0.0-dev"),
			key:      MetadataKeyClientVersion,
			expected: "0.0.0-dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractVersion(tt.md, tt.key)
			assert.Equal(t, tt.expected, got.String())
		})
	}
}

func TestCheckClientVersionHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		md         metadata.MD
		wantErr    bool
		errCode    codes.Code
		errContain string
	}{
		{
			name: "cli version below minimum",
			md: metadata.Pairs(
				MetadataKeyClientVersion, "0.0.0-dev",
			),
			wantErr:    true,
			errCode:    codes.FailedPrecondition,
			errContain: "client version is below minimum",
		},
		{
			name: "cli version above minimum",
			md: metadata.Pairs(
				MetadataKeyClientVersion, "999.0.0",
			),
			wantErr: false,
		},
		{
			name: "min daemon version above current daemon",
			md: metadata.Pairs(
				MetadataKeyClientVersion, "999.0.0",
				MetadataKeyMinServerVersion, "999.0.0",
			),
			wantErr:    true,
			errCode:    codes.FailedPrecondition,
			errContain: "daemon version",
		},
		{
			name: "min daemon version below current daemon",
			md: metadata.Pairs(
				MetadataKeyClientVersion, "999.0.0",
				MetadataKeyMinServerVersion, "0.0.1",
			),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.md)
			}

			err := checkClientVersionHeaders(ctx)

			if tt.wantErr {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok, "expected gRPC status error, got %T", err)
				assert.Equal(t, tt.errCode, st.Code())
				assert.Contains(t, st.Message(), tt.errContain)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClientInterceptors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ctx           context.Context
		wantMinServer string
	}{
		{
			name:          "default min server version",
			ctx:           context.Background(),
			wantMinServer: MinServerVersion,
		},
		{
			name:          "higher per-call min server version",
			ctx:           WithMinServerVersion(context.Background(), "999.0.0"),
			wantMinServer: "999.0.0",
		},
		{
			name:          "lower per-call min server version",
			ctx:           WithMinServerVersion(context.Background(), "0.1.0"),
			wantMinServer: MinServerVersion,
		},
		{
			name:          "highest of multiple per-call min server versions",
			ctx:           WithMinServerVersion(WithMinServerVersion(context.Background(), "999.0.0"), "998.0.0"),
			wantMinServer: "999.0.0",
		},
		{
			name: "existing outgoing metadata",
			ctx: metadata.AppendToOutgoingContext(
				WithMinServerVersion(context.Background(), "999.0.0"), "machine", "machine-1"),
			wantMinServer: "999.0.0",
		},
	}

	assertVersionMetadata := func(t *testing.T, inCtx, outCtx context.Context, wantMinServer string) {
		t.Helper()
		md, ok := metadata.FromOutgoingContext(outCtx)
		require.True(t, ok)
		assert.Equal(t, []string{currentVersion.String()}, md.Get(MetadataKeyClientVersion))
		assert.Equal(t, []string{wantMinServer}, md.Get(MetadataKeyMinServerVersion))

		// Other outgoing metadata must be preserved.
		inMD, _ := metadata.FromOutgoingContext(inCtx)
		for k, v := range inMD {
			if k != MetadataKeyMinServerVersion {
				assert.Equal(t, v, md.Get(k), "metadata key %q", k)
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var unaryCtx context.Context
			err := ClientUnaryInterceptor(tt.ctx, "/test/Unary", nil, nil, nil,
				func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
					unaryCtx = ctx
					return nil
				})
			require.NoError(t, err)
			assertVersionMetadata(t, tt.ctx, unaryCtx, tt.wantMinServer)

			var streamCtx context.Context
			_, err = ClientStreamInterceptor(tt.ctx, &grpc.StreamDesc{}, nil, "/test/Stream",
				func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
					streamCtx = ctx
					return nil, nil
				})
			require.NoError(t, err)
			assertVersionMetadata(t, tt.ctx, streamCtx, tt.wantMinServer)
		})
	}
}

func TestWithMinServerVersion_InvalidVersionPanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		WithMinServerVersion(context.Background(), "not-a-version")
	})
}
