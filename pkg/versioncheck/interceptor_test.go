package versioncheck

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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
			key:      MetadataKeyCLIVersion,
			expected: "0.0.0",
		},
		{
			name:     "missing key",
			md:       metadata.MD{},
			key:      MetadataKeyCLIVersion,
			expected: "0.0.0",
		},
		{
			name:     "empty value",
			md:       metadata.Pairs(MetadataKeyCLIVersion, ""),
			key:      MetadataKeyCLIVersion,
			expected: "0.0.0",
		},
		{
			name:     "invalid version",
			md:       metadata.Pairs(MetadataKeyCLIVersion, "not-a-version"),
			key:      MetadataKeyCLIVersion,
			expected: "0.0.0",
		},
		{
			name:     "valid version",
			md:       metadata.Pairs(MetadataKeyCLIVersion, "1.2.3"),
			key:      MetadataKeyCLIVersion,
			expected: "1.2.3",
		},
		{
			name:     "version with prerelease",
			md:       metadata.Pairs(MetadataKeyCLIVersion, "0.0.0-dev"),
			key:      MetadataKeyCLIVersion,
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
				MetadataKeyCLIVersion, "0.0.0-dev",
			),
			wantErr:    true,
			errCode:    codes.FailedPrecondition,
			errContain: "client version is below minimum",
		},
		{
			name: "cli version above minimum",
			md: metadata.Pairs(
				MetadataKeyCLIVersion, "999.0.0",
			),
			wantErr: false,
		},
		{
			name: "min daemon version above current daemon",
			md: metadata.Pairs(
				MetadataKeyCLIVersion, "999.0.0",
				MetadataKeyMinDaemonVersion, "999.0.0",
			),
			wantErr:    true,
			errCode:    codes.FailedPrecondition,
			errContain: "daemon version",
		},
		{
			name: "min daemon version below current daemon",
			md: metadata.Pairs(
				MetadataKeyCLIVersion, "999.0.0",
				MetadataKeyMinDaemonVersion, "0.0.1",
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
				assert.True(t, strings.Contains(st.Message(), tt.errContain),
					"error message = %q, want to contain %q", st.Message(), tt.errContain)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckDaemonVersionInResponse(t *testing.T) {
	tests := []struct {
		name        string
		md          metadata.MD
		wantWarning bool
	}{
		{
			name:        "daemon version below minimum",
			md:          metadata.Pairs(MetadataKeyDaemonVersion, "0.0.0-dev"),
			wantWarning: true,
		},
		{
			name:        "daemon version above minimum",
			md:          metadata.Pairs(MetadataKeyDaemonVersion, "999.0.0"),
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset warned flag for each test
			warned = false

			// Capture stderr
			old := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			checkDaemonVersionInResponse(tt.md)

			w.Close()
			var buf bytes.Buffer
			io.Copy(&buf, r)
			os.Stderr = old

			output := buf.String()

			if tt.wantWarning {
				assert.True(t, strings.Contains(output, "WARNING"), "expected warning output, got none")
			} else {
				assert.Equal(t, "", output)
			}
		})
	}
}

func TestCheckDaemonVersionInResponse_WarnOnce(t *testing.T) {
	// Reset warned flag
	warned = false

	md := metadata.Pairs(MetadataKeyDaemonVersion, "0.0.0-dev")

	// First call - should warn
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	checkDaemonVersionInResponse(md)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = old

	assert.True(t, strings.Contains(buf.String(), "WARNING"), "first call should warn")

	// Second call - should NOT warn (warned flag is now true)
	r2, w2, _ := os.Pipe()
	os.Stderr = w2

	checkDaemonVersionInResponse(md)

	w2.Close()
	var buf2 bytes.Buffer
	io.Copy(&buf2, r2)
	os.Stderr = old

	assert.Equal(t, "", buf2.String(), "second call should not warn")
}
