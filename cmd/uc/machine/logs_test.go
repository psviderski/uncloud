package machine

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/cli/logs"
	"github.com/stretchr/testify/require"
)

func TestRunLogsInvalidTimeFilters(t *testing.T) {
	t.Parallel()

	// Invalid filters must fail before connecting to the cluster.
	for _, flag := range []string{"since", "until"} {
		opts := logs.Options{}
		if flag == "since" {
			opts.Since = "invalid"
		} else {
			opts.Until = "invalid"
		}
		err := runLogs(context.Background(), nil, nil, opts)
		require.ErrorContains(t, err, "invalid --"+flag+" value")
	}
}
