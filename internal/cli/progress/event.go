package progress

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/cli/tui"
)

// PreDeployHookEventID returns a progress event identifier for pre-deploy hook operations.
func PreDeployHookEventID(serviceName, machineName string) string {
	return fmt.Sprintf("Pre-deploy hook for %s on %s", tui.NameStyle.Render(serviceName), tui.Bold.Render(machineName))
}
