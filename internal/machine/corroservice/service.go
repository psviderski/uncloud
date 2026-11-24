package corroservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/cenkalti/backoff/v4"
	"github.com/psviderski/uncloud/internal/corrosion"
)

type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	// Cleanup tears down the underlying service resources.
	Cleanup(ctx context.Context) error
	Running() bool
}

// NewAPIClient creates a client for the API of the Corrosion service configured in dataDir.
func NewAPIClient(dataDir string) (*corrosion.APIClient, error) {
	configData, err := os.ReadFile(filepath.Join(dataDir, "config.toml"))
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err = toml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	corro, err := corrosion.NewAPIClient(config.API.Addr, config.API.Authz.BearerToken)
	if err != nil {
		return nil, fmt.Errorf("create corrosion API client: %w", err)
	}
	return corro, nil
}

// WaitReady waits for the Corrosion service to be ready with the uncloud schema applied.
func WaitReady(ctx context.Context, corro *corrosion.APIClient) error {
	// Corrosion starts serving the API before applying the schema. Query the cluster table with exponential backoff
	// to check if the uncloud schema has been applied.
	checkReady := func() error {
		rows, err := corro.QueryContext(ctx, "SELECT 1 FROM cluster LIMIT 1")
		if err != nil {
			return fmt.Errorf("query cluster table: %w", err)
		}
		defer rows.Close()

		return nil
	}

	b := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(50*time.Millisecond),
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(15*time.Second),
	)
	if err := backoff.Retry(checkReady, backoff.WithContext(b, ctx)); err != nil {
		return fmt.Errorf("corrosion service did not become ready: %w", err)
	}
	return nil
}
