package compose

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/operation"
	"github.com/stretchr/testify/assert"
)

func TestPlan_FormatSummary(t *testing.T) {
	t.Parallel()

	web := &deploy.ServicePlan{}
	web.Operations = []operation.Operation{
		&operation.UpdateContainerOperation{MachineID: "m-1"},
		&operation.RemoveContainerOperation{MachineID: "m-2"},
	}
	db := &deploy.ServicePlan{}
	db.Operations = []operation.Operation{
		&operation.UpdateContainerOperation{MachineID: "m-2"},
	}
	plan := &Plan{
		Volumes:  []*operation.CreateVolumeOperation{{MachineID: "m-3"}},
		Services: []*deploy.ServicePlan{web, db},
	}

	assert.Equal(t, "1 create · 2 update (in place) · 1 remove · across 3 machines", ansi.Strip(plan.formatSummary()))
}

func TestPlan_FormatSummary_StopOperationsNotCounted(t *testing.T) {
	t.Parallel()

	svc := &deploy.ServicePlan{}
	svc.Operations = []operation.Operation{
		&operation.StopContainerOperation{MachineID: "m-1"},
		&operation.StopContainerOperation{MachineID: "m-1"},
		&operation.ReplaceContainerOperation{MachineID: "m-1", Order: api.UpdateOrderStopFirst},
		&operation.RemoveContainerOperation{MachineID: "m-1"},
	}
	plan := &Plan{Services: []*deploy.ServicePlan{svc}}

	// Stop operations aren't counted because a later operation in the plan removes the stopped container.
	assert.Equal(t, "1 replace (stop-first) · 1 remove · across 1 machine", ansi.Strip(plan.formatSummary()))
}
