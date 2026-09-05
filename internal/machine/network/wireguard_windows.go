package network

import (
	"context"
	"errors"
)

type WireGuardNetwork struct{}

func NewWireGuardNetwork() (*WireGuardNetwork, error) {
	return &WireGuardNetwork{}, nil
}

func (n *WireGuardNetwork) Configure(config Config) error {
	return errors.New("not implemented on Windows")
}

func (n *WireGuardNetwork) Run(ctx context.Context) error {
	return errors.New("not implemented on Windows")
}

func (n *WireGuardNetwork) WatchEndpoints() <-chan EndpointChangeEvent {
	return nil
}

func (n *WireGuardNetwork) Cleanup() error {
	return errors.New("not implemented on Windows")
}
