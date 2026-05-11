package compose

import (
	"github.com/compose-spec/compose-go/v2/types"
)

// ContextExtensionKey is the top-level Compose extension key for specifying the cluster context.
const ContextExtensionKey = "x-context"

// ClusterContext extracts the x-context value from the project's top-level extensions.
// Returns an empty string if x-context is not set.
func ClusterContext(project *types.Project) string {
	v, ok := project.Extensions[ContextExtensionKey]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
