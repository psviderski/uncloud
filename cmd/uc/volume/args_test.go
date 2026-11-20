package volume

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListCommandArgs(t *testing.T) {
	cmd := NewListCommand()

	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"extra"}))
}
