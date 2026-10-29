package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceContainerLogs(t *testing.T) {
	t.Parallel()

	const (
		containerID = "container-id"
		firstLog    = "2025-01-01T00:00:00.000000000Z first message\n"
		secondLog   = "2025-01-01T00:00:01.000000000Z second message\n"
	)

	var multiplexedLogs bytes.Buffer
	_, err := stdcopy.NewStdWriter(&multiplexedLogs, stdcopy.Stdout).Write([]byte(firstLog))
	require.NoError(t, err)
	_, err = stdcopy.NewStdWriter(&multiplexedLogs, stdcopy.Stderr).Write([]byte(secondLog))
	require.NoError(t, err)

	tests := []struct {
		name     string
		tty      bool
		logs     []byte
		streams  []api.LogStreamType
		messages []string
	}{
		{
			name:     "TTY raw stream",
			tty:      true,
			logs:     []byte(firstLog + secondLog),
			streams:  []api.LogStreamType{api.LogStreamStdout, api.LogStreamStdout},
			messages: []string{"first message\n", "second message\n"},
		},
		{
			name:     "non-TTY multiplexed stream",
			logs:     multiplexedLogs.Bytes(),
			streams:  []api.LogStreamType{api.LogStreamStdout, api.LogStreamStderr},
			messages: []string{"first message\n", "second message\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dockerClient := newLogsTestClient(t, tt.tty, tt.logs)
			service := NewService(dockerClient, nil)

			logsCh, err := service.ContainerLogs(context.Background(), containerID, api.ServiceLogsOptions{})
			require.NoError(t, err)

			var entries []api.LogEntry
			for entry := range logsCh {
				require.NoError(t, entry.Err)
				entries = append(entries, entry)
			}

			require.Len(t, entries, len(tt.messages))
			for i := range entries {
				assert.Equal(t, tt.streams[i], entries[i].Stream)
				assert.Equal(t, tt.messages[i], string(entries[i].Message))
				assert.False(t, entries[i].Timestamp.IsZero())
			}
		})
	}
}

func newLogsTestClient(t *testing.T, tty bool, logs []byte) *dockerclient.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/container-id/json"):
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{ID: "container-id"},
				Config:            &container.Config{Tty: tty},
			}); err != nil {
				t.Errorf("encode inspect response: %v", err)
			}
		case strings.HasSuffix(r.URL.Path, "/containers/container-id/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			if _, err := w.Write(logs); err != nil {
				t.Errorf("write logs response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	dockerClient, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost("tcp://"+serverURL.Host),
		dockerclient.WithHTTPClient(server.Client()),
		dockerclient.WithVersion("1.48"),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, dockerClient.Close())
	})

	return dockerClient
}
