package cli

import (
	"fmt"
	"uncloud/internal/cli/config"
)

type CLI struct {
	config *config.Config
}

func New(configPath string) (*CLI, error) {
	cfg, err := config.NewFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read Uncloud config: %w", err)
	}
	return &CLI{
		config: cfg,
	}, nil
}
