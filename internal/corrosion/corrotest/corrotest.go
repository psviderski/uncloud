// Package corrotest runs Corrosion in a Docker container for tests.
package corrotest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/psviderski/uncloud/internal/corrosion"
	"github.com/psviderski/uncloud/internal/machine/corroservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

const (
	dataDir = "/data"
	apiPort = "8080/tcp"
)

// Corrosion is a single-node Corrosion running in a Docker container.
type Corrosion struct {
	Client    *corrosion.APIClient
	container *testcontainers.DockerContainer
}

// Start starts a single-node Corrosion with the same image as uncloudd in a Docker container and waits until
// it applies the schema. The schema must define the cluster table like the uncloud schema in store.Schema does.
// The caller must terminate the returned Corrosion. If Start fails, it removes the container itself.
func Start(ctx context.Context, schema string) (*Corrosion, error) {
	cfg := corroservice.Config{
		DB: corroservice.DBConfig{
			Path:        dataDir + "/store.db",
			SchemaPaths: []string{dataDir + "/schema.sql"},
		},
		Gossip: corroservice.GossipConfig{
			Addr:      netip.MustParseAddrPort("127.0.0.1:8787"),
			Plaintext: true,
		},
		API: corroservice.APIConfig{
			// Listen on all interfaces to be reachable through the published port.
			Addr:  netip.MustParseAddrPort("0.0.0.0:8080"),
			Authz: corroservice.APIAuthzConfig{BearerToken: rand.Text()},
		},
		Admin: corroservice.AdminConfig{Path: dataDir + "/admin.sock"},
	}
	config, err := cfg.Marshal()
	if err != nil {
		return nil, err
	}

	ctr, err := testcontainers.Run(ctx, corroservice.Image,
		testcontainers.WithCmd("corrosion", "agent", "-c", dataDir+"/config.toml"),
		testcontainers.WithExposedPorts(apiPort),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				Reader:            bytes.NewReader(config),
				ContainerFilePath: dataDir + "/config.toml",
				FileMode:          0o600,
			},
			testcontainers.ContainerFile{
				Reader:            strings.NewReader(schema),
				ContainerFilePath: dataDir + "/schema.sql",
				FileMode:          0o644,
			},
		),
	)
	if err != nil {
		return nil, removeFailed(ctr, fmt.Errorf("start Corrosion container: %w", err))
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, removeFailed(ctr, fmt.Errorf("get Docker host: %w", err))
	}
	port, err := ctr.MappedPort(ctx, apiPort)
	if err != nil {
		return nil, removeFailed(ctr, fmt.Errorf("get published API port: %w", err))
	}
	// The API client needs an IP address but the Docker host is usually a name like localhost.
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err == nil && len(ips) == 0 {
		err = errors.New("no IPv4 address found")
	}
	if err != nil {
		return nil, removeFailed(ctr, fmt.Errorf("resolve Docker host '%s': %w", host, err))
	}

	client, err := corrosion.NewAPIClient(netip.AddrPortFrom(ips[0], uint16(port.Int())), cfg.API.Authz.BearerToken)
	if err != nil {
		return nil, removeFailed(ctr, err)
	}
	if err = corroservice.WaitReady(ctx, client); err != nil {
		return nil, removeFailed(ctr, err)
	}

	return &Corrosion{Client: client, container: ctr}, nil
}

// Terminate stops and removes the Corrosion container.
func (c *Corrosion) Terminate(ctx context.Context) error {
	return c.container.Terminate(ctx)
}

// logOnFailure logs the Corrosion output if the test fails.
func (c *Corrosion) logOnFailure(t testing.TB) {
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		logs, err := containerLogs(c.container)
		if err != nil {
			t.Logf("Failed to get Corrosion container logs: %v", err)
			return
		}
		t.Logf("Corrosion container logs:\n%s", logs)
	})
}

// Run runs Corrosion like [Start] for a single test and returns a client for its API. The container is removed
// when the test completes.
func Run(t testing.TB, schema string) *corrosion.APIClient {
	t.Helper()

	corro, err := Start(t.Context(), schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, corro.Terminate(context.Background()), "terminate Corrosion container")
	})
	// Registered after the termination so the logs are read before the container is removed.
	corro.logOnFailure(t)

	return corro.Client
}

// Shared is a Corrosion that tests can share, e.g. all tests in a package. It starts on the first Start call and runs
// until Terminate, which is usually called from TestMain after the tests have run. Tests see each other's data.
type Shared struct {
	// Schema is the database schema like for [Start].
	Schema string

	once  sync.Once
	corro *Corrosion
	err   error
}

// Client returns a client for the shared Corrosion API, starting Corrosion on the first call. It fails the test
// if Corrosion fails to start.
func (s *Shared) Client(t testing.TB) *corrosion.APIClient {
	t.Helper()

	s.once.Do(func() {
		// Don't use the test context because Corrosion outlives the test that starts it.
		s.corro, s.err = Start(context.Background(), s.Schema)
	})
	require.NoError(t, s.err, "start shared Corrosion")
	s.corro.logOnFailure(t)

	return s.corro.Client
}

// Terminate stops and removes the shared Corrosion container. It's a no-op if Corrosion hasn't started.
func (s *Shared) Terminate(ctx context.Context) error {
	if s.corro == nil {
		return nil
	}
	return s.corro.Terminate(ctx)
}

// removeFailed removes a container that failed to become ready and adds the container logs to err.
func removeFailed(ctr *testcontainers.DockerContainer, err error) error {
	if ctr == nil {
		return err
	}
	if logs, lErr := containerLogs(ctr); lErr == nil {
		err = fmt.Errorf("%w\nCorrosion container logs:\n%s", err, logs)
	}
	return errors.Join(err, testcontainers.TerminateContainer(ctr))
}

// containerLogs returns the stdout and stderr output of the container.
func containerLogs(ctr *testcontainers.DockerContainer) (string, error) {
	rc, err := ctr.Logs(context.Background())
	if err != nil {
		return "", err
	}
	defer rc.Close()

	logs, err := io.ReadAll(rc)
	return string(logs), err
}
