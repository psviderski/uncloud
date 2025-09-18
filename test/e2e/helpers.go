package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/psviderski/uncloud/internal/ucind"
	"github.com/stretchr/testify/assert"
)

type fileInfo struct {
	permissions os.FileMode
	content     string
}

// a helper function that takes a machine, container name inside it and file path, and returns the file contents along with metadata
//
// Uncloud API does not currently expose exec functionality to run commands inside containers.
// Instead this helper function uses "docker cp" inside the ucind container to copy the file from the target container
// to a temporary location (also inside the ucind container), and then inspect its content and permissions.
func readFileInfoInContainer(t *testing.T, machine *ucind.Machine, containerName, filePath string) (fileInfo, error) {
	t.Helper()
	ctx := context.Background()

	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return fileInfo{}, err
	}
	defer dockerCli.Close()

	machineContainerName := fmt.Sprintf("%s-%s", machine.ClusterName, machine.Name)

	//  1. Create an exec instance
	fileLocation := fmt.Sprintf("%s:%s", containerName, filePath)
	tmpFileLocation := fmt.Sprintf("/tmp/%s-%s", containerName, filepath.Base(filePath))

	cmdDocker := fmt.Sprintf("docker cp %s %s", fileLocation, tmpFileLocation)
	cmdPermissions := fmt.Sprintf("stat -c %%a %s", tmpFileLocation)
	cmdCat := fmt.Sprintf("cat %s", tmpFileLocation)
	cmdCombined := fmt.Sprintf("%s; %s; %s", cmdDocker, cmdPermissions, cmdCat)

	execConfig := container.ExecOptions{
		Cmd:          []string{"sh", "-ec", cmdCombined},
		AttachStdout: true,
		AttachStderr: true,
	}
	resp, err := dockerCli.ContainerExecCreate(ctx, machineContainerName, execConfig)
	if err != nil {
		return fileInfo{}, err
	}

	// 2. Attach to the exec session
	hijackResp, err := dockerCli.ContainerExecAttach(ctx, resp.ID, container.ExecAttachOptions{})
	if err != nil {
		return fileInfo{}, err
	}
	defer hijackResp.Close()

	// 3. Inspect result
	inspectResp, err := dockerCli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return fileInfo{}, err
	}

	assert.Equal(t, 0, inspectResp.ExitCode, "Expected exit code 0 from exec command")

	// 4. Read output to string using stdcopy to demultiplex the Docker stream
	var stdout, stderr strings.Builder
	_, err = stdcopy.StdCopy(&stdout, &stderr, hijackResp.Reader)
	if err != nil {
		return fileInfo{}, err
	}

	if stderr.Len() > 0 {
		return fileInfo{}, fmt.Errorf("stderr output: %s", stderr.String())
	}

	// 5. Parse output: permissions, content
	outputLines := strings.SplitN(stdout.String(), "\n", 2)
	if len(outputLines) < 2 {
		return fileInfo{}, fmt.Errorf("unexpected output format, expected at least 2 lines, got %d", len(outputLines))
	}
	permissions := outputLines[0]
	// convert to octal number
	permissionsOctal, err := strconv.ParseInt(permissions, 8, 32)
	if err != nil {
		return fileInfo{}, fmt.Errorf("parse file permissions: %w", err)
	}
	mode := os.FileMode(permissionsOctal)

	fileContent := outputLines[1]

	return fileInfo{
		permissions: mode,
		content:     fileContent,
	}, nil
}
