package compose

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
)

const PreDeployHookExtensionKey = "x-pre_deploy"

// PreDeployHook represents the parsed x-pre_deploy extension config.
type PreDeployHook struct {
	Command     types.ShellCommand      `yaml:"command" json:"command"`
	Environment types.MappingWithEquals `yaml:"environment,omitempty" json:"environment,omitempty"`
	Privileged  *bool                   `yaml:"privileged,omitempty" json:"privileged,omitempty"`
	Timeout     *types.Duration         `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	User        string                  `yaml:"user,omitempty" json:"user,omitempty"`
}

// Validate checks that the pre-deploy hook configuration is valid.
func (p *PreDeployHook) Validate() error {
	if len(p.Command) == 0 {
		return fmt.Errorf("missing required attribute 'command' in %s extension", PreDeployHookExtensionKey)
	}
	return nil
}
