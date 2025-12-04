package e2e

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deployTestService is a helper function to deploy a simple alpine service for testing exec
func deployTestService(t *testing.T, ctx context.Context, cli *client.Client, name string, replicas uint) {
	t.Helper()

	spec := api.ServiceSpec{
		Name:     name,
		Replicas: replicas,
		Container: api.ContainerSpec{
			Image:   "alpine:3.20",
			Command: []string{"sleep", "3600"},
		},
	}

	deployment := cli.NewDeployment(spec, nil)
	_, err := deployment.Run(ctx)
	require.NoError(t, err)

	// Wait for all replicas to be running
	require.Eventually(t, func() bool {
		service, err := cli.InspectService(ctx, name)
		if err != nil {
			return false
		}
		assertServiceMatchesSpec(t, service, spec)

		for _, ctr := range service.Containers {
			if ctr.Container.State.Status != "running" {
				return false
			}
		}
		return true
	}, 30*time.Second, 1*time.Second, fmt.Sprintf("service %s should have %d running replicas", name, replicas))
}

// TestExecBasicCommands tests basic command execution, errors, and stderr handling
func TestExecBasicCommands(t *testing.T) {
	t.Parallel()

	clusterName := "ucind-test.exec-basic"
	ctx := context.Background()
	c, _ := createTestCluster(t, clusterName, ucind.CreateClusterOptions{Machines: 1}, true)

	cli, err := c.Machines[0].Connect(ctx)
	require.NoError(t, err)

	// Test: Non-existent service should fail
	t.Run("non-existent service", func(t *testing.T) {
		execOptions := api.ExecOptions{
			Command:      []string{"echo", "test"},
			AttachStdout: true,
			AttachStderr: true,
		}

		_, err := cli.ExecContainer(ctx, "non-existent-service", "", execOptions)
		assert.Error(t, err, "should fail for non-existent service")
		assert.Contains(t, strings.ToLower(err.Error()), "inspect service: not found")
	})

	// Deploy a simple service for all remaining tests
	serviceName := "test-exec-service"
	deployTestService(t, ctx, cli, serviceName, 1)

	// Test: Execute a simple echo command
	t.Run("echo command", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		execOptions := api.ExecOptions{
			Command:      []string{"echo", "hello world"},
			AttachStdout: true,
			AttachStderr: true,
			Stdout:       &stdout,
			Stderr:       &stderr,
		}

		exitCode, err := cli.ExecContainer(ctx, serviceName, "", execOptions)
		require.NoError(t, err)
		assert.Equal(t, 0, exitCode, "command should exit with code 0")
		assert.Equal(t, "hello world\n", stdout.String(), "should capture stdout")
		assert.Empty(t, stderr.String(), "stderr should be empty")
	})

	// Test: Command with non-zero exit code
	t.Run("non-zero exit code", func(t *testing.T) {
		execOptions := api.ExecOptions{
			Command:      []string{"sh", "-c", "exit 42"},
			AttachStdout: true,
			AttachStderr: true,
		}

		exitCode, err := cli.ExecContainer(ctx, serviceName, "", execOptions)
		require.NoError(t, err)
		assert.Equal(t, 42, exitCode, "should return actual exit code")
	})

	// Test: Invalid command should fail
	t.Run("invalid command", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		execOptions := api.ExecOptions{
			Command:      []string{"nonexistent-command"},
			AttachStdout: true,
			AttachStderr: true,
			Stdout:       &stdout,
			Stderr:       &stderr,
		}

		exitCode, err := cli.ExecContainer(ctx, serviceName, "", execOptions)
		require.NoError(t, err, "exec should not fail, but command should return non-zero exit")
		assert.Equal(t, 126, exitCode, "invalid command should return non-zero exit code")
		assert.Contains(t, stdout.String(), "executable file not found")
		assert.Equal(t, "", stderr.String(), "stderr should be empty")
	})

	// Test: Non-existent container ID
	t.Run("non-existent container", func(t *testing.T) {
		execOptions := api.ExecOptions{
			Command:      []string{"echo", "test"},
			AttachStdout: true,
			AttachStderr: true,
		}

		_, err := cli.ExecContainer(ctx, serviceName, "non-existent-container-id", execOptions)
		assert.Error(t, err, "should fail for non-existent container")
	})

	// Test: Empty command
	t.Run("empty command", func(t *testing.T) {
		execOptions := api.ExecOptions{
			Command:      []string{},
			AttachStdout: true,
			AttachStderr: true,
		}

		_, err := cli.ExecContainer(ctx, serviceName, "", execOptions)
		assert.Error(t, err, "should fail for empty command")
	})

	// Test: Command with both stdout and stderr
	t.Run("mixed stdout/stderr", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		execOptions := api.ExecOptions{
			Command:      []string{"sh", "-c", "echo 'stdout'; echo 'stderr' >&2"},
			AttachStdout: true,
			AttachStderr: true,
			Stdout:       &stdout,
			Stderr:       &stderr,
		}

		exitCode, err := cli.ExecContainer(ctx, serviceName, "", execOptions)
		require.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		assert.Equal(t, "stdout\n", stdout.String(), "should capture stdout")
		assert.Equal(t, "stderr\n", stderr.String(), "should capture stderr")
	})

	// Test: Detached command should return immediately
	t.Run("detached command", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		start := time.Now()
		execOptions := api.ExecOptions{
			Command: []string{"sh", "-c", "sleep 10; echo hello"}, // Long-running command
			Stdout:  &stdout,
			Stderr:  &stderr,
			Detach:  true,
		}

		exitCode, err := cli.ExecContainer(ctx, serviceName, "", execOptions)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		// Should return immediately, not wait for command to complete
		assert.Less(t, elapsed, 5*time.Second, "detached command should return quickly")
		assert.Empty(t, stdout.String(), "stdout should be empty for detached command")
		assert.Empty(t, stderr.String(), "stderr should be empty for detached command")
	})

	// Test: Detached mode with invalid command
	t.Run("detached invalid command", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		execOptions := api.ExecOptions{
			Command: []string{"nonexistent-detached-command"},
			Stdout:  &stdout,
			Stderr:  &stderr,
			Detach:  true,
		}

		exitCode, err := cli.ExecContainer(ctx, serviceName, "", execOptions)

		require.ErrorContains(t, err, "executable file not found")
		assert.Equal(t, 1, exitCode)
		assert.Empty(t, stdout.String(), "stdout should be empty for detached command")
		assert.Empty(t, stderr.String(), "stderr should be empty for detached command")
	})

	// Deploy a service with multiple replicas
	multiServiceName := "multi-replica-service"
	deployTestService(t, ctx, cli, multiServiceName, 2)

	// Test: Execute command on specific container
	t.Run("exec on specific container", func(t *testing.T) {
		service, err := cli.InspectService(ctx, multiServiceName)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(service.Containers), 2, "should have at least 2 containers")

		var stdout, stderr bytes.Buffer

		// Use container prefix
		containerIDPrefix := service.Containers[1].Container.ID[:3]
		execOptions := api.ExecOptions{
			Command:      []string{"hostname"},
			AttachStdout: true,
			AttachStderr: true,
			Stdout:       &stdout,
			Stderr:       &stderr,
		}

		exitCode, err := cli.ExecContainer(ctx, multiServiceName, containerIDPrefix, execOptions)

		require.NoError(t, err)
		assert.Equal(t, 0, exitCode)
		assert.Greater(t, len(stdout.String()), 4)
		assert.Equal(t, service.Containers[1].Container.Name+"\n", stdout.String())
	})
}
