package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/stretchr/testify/assert"
)

type fileInfo struct {
	permissions os.FileMode
	content     string
	userId      int
	groupId     int
}

// execInContainerAndReadOutput is a helper that executes a command in a container and returns stdout output.
// It validates the exit code is 0 and that there's no stderr output.
func execInContainerAndReadOutput(
	t *testing.T,
	ctx context.Context,
	cli *client.Client,
	serviceNameOrID, containerNameOrID string,
	command []string,
) (string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	execOpts := api.ExecOptions{
		Command:      command,
		AttachStdout: true,
		AttachStderr: true,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	commandName := command[0]

	exitCode, err := cli.ExecContainer(ctx, serviceNameOrID, containerNameOrID, execOpts)
	if err != nil {
		return "", fmt.Errorf("exec %s: %w", commandName, err)
	}

	assert.Equal(t, 0, exitCode, "Expected exit code 0 from %s command", commandName)

	if stderr.Len() > 0 {
		return "", fmt.Errorf("%s stderr output: %s", commandName, stderr.String())
	}

	return stdout.String(), nil
}

// readFileInfoInContainer reads file information from a container using the Uncloud client ExecContainer API.
// Information read: permissions, uid, gid, content.
func readFileInfoInContainer(t *testing.T, cli *client.Client, serviceNameOrID, containerNameOrID, filePath string) (fileInfo, error) {
	t.Helper()
	ctx := context.Background()

	// Get file permissions
	permOutput, err := execInContainerAndReadOutput(t, ctx, cli, serviceNameOrID, containerNameOrID,
		[]string{"stat", "-c", "%a %u %g", filePath})
	if err != nil {
		return fileInfo{}, err
	}

	// Parse permissions
	permissions := strings.TrimSpace(permOutput)

	// Parse three numbers: permissions, uid, gid
	var permissionsOctal, uid, gid int
	_, err = fmt.Sscanf(permissions, "%o %d %d", &permissionsOctal, &uid, &gid)
	if err != nil {
		return fileInfo{}, fmt.Errorf("parse stat output: %w", err)
	}

	mode := os.FileMode(permissionsOctal)

	// Get file content
	fileContent, err := execInContainerAndReadOutput(t, ctx, cli, serviceNameOrID, containerNameOrID,
		[]string{"cat", filePath})
	if err != nil {
		return fileInfo{}, err
	}

	return fileInfo{
		permissions: mode,
		userId:      uid,
		groupId:     gid,
		content:     fileContent,
	}, nil
}
