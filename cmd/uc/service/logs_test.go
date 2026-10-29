package service

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/cli/logs"
	"github.com/stretchr/testify/require"
)

func TestRunLogsInvalidTimeFilters(t *testing.T) {
	t.Parallel()

	// Invalid filters must fail before loading Compose files or connecting to the cluster.
	for _, flag := range []string{"since", "until"} {
		opts := logs.Options{}
		if flag == "since" {
			opts.Since = "invalid"
		} else {
			opts.Until = "invalid"
		}
		err := RunLogs(context.Background(), nil, nil, opts)
		require.ErrorContains(t, err, "invalid --"+flag+" value")
	}
}
