package grpcversion

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestParseVersionOrZero(t *testing.T) {
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

func captureWarnings(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := WarnWriter
	WarnWriter = &buf
	t.Cleanup(func() { WarnWriter = old })
	fn()
	return buf.String()
}

func TestCheckServerVersionInResponse(t *testing.T) {
	tests := []struct {
		name        string
		md          metadata.MD
		wantWarning bool
	}{
		{
			name:        "daemon version below minimum",
			md:          metadata.Pairs(MetadataKeyServerVersion, "0.0.0-dev"),
			wantWarning: true,
		},
		{
			name:        "daemon version above minimum",
			md:          metadata.Pairs(MetadataKeyServerVersion, "999.0.0"),
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warned.Store(false)

			output := captureWarnings(t, func() {
				checkServerVersionInResponse(tt.md)
			})

			if tt.wantWarning {
				assert.Contains(t, output, "WARNING")
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestCheckServerVersionInResponse_WarnOnce(t *testing.T) {
	warned.Store(false)

	md := metadata.Pairs(MetadataKeyServerVersion, "0.0.0-dev")

	// First call should warn.
	output1 := captureWarnings(t, func() {
		checkServerVersionInResponse(md)
	})
	assert.Contains(t, output1, "WARNING", "first call should warn")

	// Second call should not warn (warned flag is now true).
	output2 := captureWarnings(t, func() {
		checkServerVersionInResponse(md)
	})
	assert.Empty(t, output2, "second call should not warn")
}
