package e2e

import (
	"os"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
)

func TestMain(m *testing.M) {
	// Disable the default health monitor period to speed up tests. Tests that need a non-zero monitor period
	// should set it explicitly via UpdateConfig.MonitorPeriod in their service spec.
	api.DefaultHealthMonitorPeriod = 0
	os.Exit(m.Run())
}
