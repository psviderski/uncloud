package deploy

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/operation"
	"github.com/stretchr/testify/assert"
)

func TestServicePlan_FormatSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ops      []operation.Operation
		expected string
	}{
		{
			name: "in-place updates only",
			ops: []operation.Operation{
				&operation.UpdateContainerOperation{MachineID: "m-1"},
				&operation.UpdateContainerOperation{MachineID: "m-2"},
			},
			expected: "2 update (in place) · across 2 machines",
		},
		{
			name: "all operation types",
			ops: []operation.Operation{
				&operation.RunContainerOperation{MachineID: "m-1"},
				&operation.ReplaceContainerOperation{MachineID: "m-1", Order: api.UpdateOrderStartFirst},
				&operation.ReplaceContainerOperation{MachineID: "m-2", Order: api.UpdateOrderStopFirst},
				&operation.UpdateContainerOperation{MachineID: "m-2"},
				&operation.RemoveContainerOperation{MachineID: "m-3"},
			},
			expected: "1 create · 1 replace (start-first) · 1 replace (stop-first) · 1 update (in place) · 1 remove · " +
				"across 3 machines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := ServicePlan{}
			plan.Operations = tt.ops
			assert.Equal(t, tt.expected, ansi.Strip(plan.FormatSummary()))
		})
	}
}
