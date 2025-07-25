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
	Running() bool
}

// WaitReady waits for the Corrosion service to be ready with the uncloud schema applied.
func WaitReady(ctx context.Context, dataDir string) error {
	// Read the config file to get the API address.
	configPath := filepath.Join(dataDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err = toml.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	corro, err := corrosion.NewAPIClient(config.API.Addr)
	if err != nil {
		return fmt.Errorf("create corrosion API client: %w", err)
	}

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
	if err = backoff.Retry(checkReady, backoff.WithContext(b, ctx)); err != nil {
		return fmt.Errorf("corrosion service did not become ready: %w", err)
	}
	return nil
}
