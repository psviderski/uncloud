package compose

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
)

// Plan holds the compose-level deployment plan with volume and service operations.
type Plan struct {
	Volumes  []*operation.CreateVolumeOperation
	Services []*deploy.ServicePlan
}

// IsEmpty returns true if the plan has no volume or service operations.
func (p *Plan) IsEmpty() bool {
	return len(p.Volumes) == 0 && len(p.Services) == 0
}

// Format renders the entire deployment plan as a styled tree with a summary footer.
func (p *Plan) Format(resolvers map[string]operation.NameResolver) string {
	var out strings.Builder

	// Format volume operations.
	for _, op := range p.Volumes {
		out.WriteString(op.Format(nil))
		out.WriteString("\n")
	}
	if len(p.Volumes) > 0 {
		out.WriteString("\n")
	}

	// Format service plans.
	for _, svcPlan := range p.Services {
		resolver := resolvers[svcPlan.ServiceID]
		out.WriteString(svcPlan.Format(resolver))
		out.WriteString("\n\n")
	}

	// Format summary footer.
	summary := p.formatSummary()
	out.WriteString(tui.Faint.Render(strings.Repeat("─", lipgloss.Width(summary))))
	out.WriteString("\n")
	out.WriteString(summary)
	out.WriteString("\n")

	return out.String()
}

// formatSummary counts all operations across the plan and renders the summary footer.
func (p *Plan) formatSummary() string {
	var createCount, startFirstCount, stopFirstCount, removeCount int
	machines := make(map[string]struct{})

	for _, op := range p.Volumes {
		machines[op.MachineID] = struct{}{}
		createCount++
	}

	for _, svcPlan := range p.Services {
		for _, op := range svcPlan.Operations {
			switch o := op.(type) {
			case *operation.RunContainerOperation:
				machines[o.MachineID] = struct{}{}
				createCount++
			case *operation.ReplaceContainerOperation:
				machines[o.MachineID] = struct{}{}
				if o.Order == api.UpdateOrderStopFirst {
					stopFirstCount++
				} else {
					startFirstCount++
				}
			case *operation.RemoveContainerOperation:
				machines[o.MachineID] = struct{}{}
				removeCount++
			case *operation.StopContainerOperation:
				machines[o.MachineID] = struct{}{}
				removeCount++
			}
		}
	}

	var parts []string
	if createCount > 0 {
		parts = append(parts,
			tui.BoldGreen.Render(strconv.Itoa(createCount))+" "+tui.Green.Render("create"))
	}
	if startFirstCount > 0 {
		parts = append(parts,
			tui.BoldGreen.Render(strconv.Itoa(startFirstCount))+" "+tui.Green.Render("replace (start-first)"))
	}
	if stopFirstCount > 0 {
		parts = append(parts,
			tui.BoldYellow.Render(strconv.Itoa(stopFirstCount))+" "+tui.Yellow.Render("replace (stop-first)"))
	}
	if removeCount > 0 {
		parts = append(parts,
			tui.BoldRed.Render(strconv.Itoa(removeCount))+" "+tui.Red.Render("remove"))
	}

	machinesWord := "machines"
	if len(machines) == 1 {
		machinesWord = "machine"
	}
	parts = append(parts, fmt.Sprintf("across %s %s", tui.Bold.Render(strconv.Itoa(len(machines))), machinesWord))

	sep := " " + tui.Faint.Render("·") + " "
	return strings.Join(parts, sep)
}

// Execute runs all volume operations followed by all service operations.
func (p *Plan) Execute(ctx context.Context, cli operation.Client) error {
	for _, op := range p.Volumes {
		if err := op.Execute(ctx, cli); err != nil {
			return err
		}
	}
	for _, sp := range p.Services {
		if err := sp.Execute(ctx, cli); err != nil {
			return err
		}
	}
	return nil
}
