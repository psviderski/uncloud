package caddyconfig

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestService_LastReconciliationError(t *testing.T) {
	service := NewService(t.TempDir())
	assert.NoError(t, service.LastReconciliationError())

	service.setReconciliationResult(errors.New("global Caddy config rejected"))
	assert.EqualError(t, service.LastReconciliationError(), "global Caddy config rejected")

	service.setReconciliationResult(nil)
	assert.NoError(t, service.LastReconciliationError())
}

func TestService_LastReconciliationErrorConcurrent(t *testing.T) {
	service := NewService(t.TempDir())
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 100 {
				service.setReconciliationResult(errors.New("retry"))
				service.setReconciliationResult(nil)
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				_ = service.LastReconciliationError()
			}
		}()
	}
	wg.Wait()
}
