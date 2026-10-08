package caddyconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestServer_GetConfig(t *testing.T) {
	t.Run("returns saved Caddyfile and reconciliation error", func(t *testing.T) {
		dir := t.TempDir()
		caddyfile := "example.com { respond ok }\n"
		path := filepath.Join(dir, "Caddyfile")
		require.NoError(t, os.WriteFile(path, []byte(caddyfile), 0o640))
		info, err := os.Stat(path)
		require.NoError(t, err)

		service := NewService(dir)
		service.setReconciliationResult(errors.New("module not registered: caddy.storage.uncloud"))
		config, err := NewServer(service).GetConfig(context.Background(), &emptypb.Empty{})

		require.NoError(t, err)
		assert.Equal(t, caddyfile, config.GetCaddyfile())
		assert.True(t, info.ModTime().Equal(config.GetModifiedAt().AsTime()))
		assert.Equal(t, "module not registered: caddy.storage.uncloud", config.GetLastReconciliationError())

		service.setReconciliationResult(nil)
		config, err = NewServer(service).GetConfig(context.Background(), &emptypb.Empty{})
		require.NoError(t, err)
		assert.Empty(t, config.GetLastReconciliationError())
	})

	t.Run("returns NotFound with reconciliation error when Caddyfile is absent", func(t *testing.T) {
		service := NewService(t.TempDir())
		service.setReconciliationResult(errors.New("module not registered: caddy.storage.uncloud"))

		config, err := NewServer(service).GetConfig(context.Background(), &emptypb.Empty{})

		assert.Nil(t, config)
		assert.Equal(t, codes.NotFound, status.Code(err))
		assert.ErrorContains(t, err, "Caddyfile")
		assert.ErrorContains(t, err, "last Caddy config load failed: module not registered: caddy.storage.uncloud")
	})

	t.Run("returns NotFound without reconciliation error when Caddyfile is absent", func(t *testing.T) {
		service := NewService(t.TempDir())

		config, err := NewServer(service).GetConfig(context.Background(), &emptypb.Empty{})

		assert.Nil(t, config)
		assert.Equal(t, codes.NotFound, status.Code(err))
		assert.NotContains(t, err.Error(), "last Caddy config load failed")
	})
}
