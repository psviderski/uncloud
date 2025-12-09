package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/psviderski/uncloud/pkg/api"
)

// ApplyNamespaceOverride sets the namespace extension for all services in the project.
func ApplyNamespaceOverride(project *types.Project, namespace string) error {
	if err := api.ValidateNamespaceName(namespace); err != nil {
		return fmt.Errorf("invalid namespace: %w", err)
	}

	for i, svc := range project.Services {
		if svc.Extensions == nil {
			svc.Extensions = make(types.Extensions)
		}
		svc.Extensions[NamespaceExtensionKey] = NamespaceSource(namespace)
		project.Services[i] = svc
	}
	return nil
}
