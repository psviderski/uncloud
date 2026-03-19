package deploy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/operation"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
)

type Client interface {
	api.ContainerClient
	api.DNSClient
	api.ImageClient
	api.MachineClient
	api.ServiceClient
	api.VolumeClient
}

// Deployment manages the process of creating or updating a service to match a desired state.
// It coordinates the validation, planning, and execution of deployment operations.
type Deployment struct {
	Service  *api.Service
	Spec     api.ServiceSpec
	Strategy Strategy
	cli      Client
	plan     *ServicePlan
	// state is an optional current and planned cluster state used for scheduling decisions.
	state *scheduler.ClusterState
}

type ServicePlan struct {
	ServiceID   string
	ServiceName string
	// Spec is the desired service spec being deployed.
	Spec api.ServiceSpec
	operation.SequenceOperation
}

// Format renders the service plan as a styled block with a spec diff and nested container operations.
func (sp *ServicePlan) Format() string {
	// Determine service-level operation type and extract the old spec from container operations.
	// Assume replace operations precede remove operations (rolling strategy) so the first replace operation
	// (if exists) determines the old spec for the diff. Otherwise, fallback to the first remove operation.
	var hasRun, hasReplace, hasRemove bool
	var oldSpec *api.ServiceSpec
	for _, op := range sp.Operations {
		switch o := op.(type) {
		case *operation.RunContainerOperation:
			hasRun = true
		case *operation.ReplaceContainerOperation:
			hasReplace = true
			if oldSpec == nil {
				oldSpec = &o.OldContainer.ServiceSpec
			}
		case *operation.RemoveContainerOperation:
			hasRemove = true
			if oldSpec == nil {
				oldSpec = &o.Container.ServiceSpec
			}
		}
	}

	// Service line modifier and verb.
	var modifier, verb string
	switch {
	case hasRun && !hasReplace && !hasRemove:
		modifier = tui.BoldGreen.Render("+")
		verb = "create"
	// TODO: when service removal via a deployment is supported, handle "remove" verb here as well.
	default:
		modifier = tui.BoldYellow.Render("~")
		verb = "update"
	}

	var out strings.Builder
	line := modifier + " " + verb + " service " + tui.NameStyle.Render(sp.ServiceName)
	if sp.Spec.Mode == api.ServiceModeGlobal {
		line += " " + tui.Faint.Render("(global)")
	}
	out.WriteString(line)
	out.WriteString("\n")

	// Build spec diff table: columns are [modifier, attribute, value or change].
	// TODO: print diff for all changed attributes, not just image and replicas.
	//  Consider reusing the logic in EvalContainerSpecChange to return a structured diff.
	specTable := table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).BorderBottom(false).
		BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch col {
			case 0: // Modifier column.
				return tui.Yellow.Width(2)
			case 1: // Attribute column.
				return tui.Faint.PaddingRight(1)
			default:
				return lipgloss.NewStyle()
			}
		})

	// Image row.
	if oldSpec == nil {
		specTable.Row("", "image:", formatImageDiff("", sp.Spec.Container.Image))
	} else {
		mod := ""
		if oldSpec.Container.Image != sp.Spec.Container.Image {
			mod = "~"
		}
		specTable.Row(mod, "image:", formatImageDiff(oldSpec.Container.Image, sp.Spec.Container.Image))
	}

	// Replicas row for replicated services.
	if sp.Spec.Mode == api.ServiceModeReplicated {
		replicasStr := fmt.Sprintf("%d", sp.Spec.Replicas)
		if oldSpec == nil {
			specTable.Row("", "replicas:", tui.Green.Render(replicasStr))
		} else if sp.Spec.Replicas > 1 || hasRun || hasRemove {
			mod := ""
			if hasRun || hasRemove {
				mod = "~"
				replicasStr = tui.Green.Render(replicasStr)
			}
			specTable.Row(mod, "replicas:", replicasStr)
		}
	}

	// Stack "  │ " tree prefixes vertically, then join horizontally with the table.
	tableStr := specTable.String()
	treePrefix := tui.Faint.Render("  │ ")
	treeColRows := make([]string, specTable.GetData().Rows())
	for i := range treeColRows {
		treeColRows[i] = treePrefix
	}
	treeCol := lipgloss.JoinVertical(lipgloss.Left, treeColRows...)

	out.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, treeCol, tableStr))
	out.WriteString("\n")

	// Blank separator line before container operations.
	out.WriteString(tui.Faint.Render("  │"))
	out.WriteString("\n")

	// Format each container operation.
	opsCount := len(sp.Operations)
	for i, op := range sp.Operations {
		connector := tui.Faint.Render("  ├──")
		if i == opsCount-1 {
			connector = tui.Faint.Render("  ╰──")
		}
		out.WriteString(connector + " " + op.Format())
		out.WriteString("\n")
	}

	return out.String()
}

// FormatSummary counts operations in the service plan and renders a styled summary line.
func (sp *ServicePlan) FormatSummary() string {
	var createCount, startFirstCount, stopFirstCount, removeCount int
	machines := make(map[string]struct{})

	for _, op := range sp.Operations {
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

// formatImageDiff formats the image for display. If oldImage is empty, it formats newImage as a new (green) value.
// Otherwise, it renders the diff between oldImage and newImage.
func formatImageDiff(oldImage, newImage string) string {
	newRef, _ := reference.ParseDockerRef(newImage) // ignore error since the image was already validated

	// Create case: no old image.
	if oldImage == "" {
		return tui.FormatImage(newRef, tui.Green)
	}

	// Update case: no change.
	if oldImage == newImage {
		return tui.FormatImage(newRef, lipgloss.NewStyle())
	}

	oldRef, _ := reference.ParseDockerRef(oldImage)

	// If either uses a digest, show full old → new.
	_, oldDigested := oldRef.(reference.Digested)
	_, newDigested := newRef.(reference.Digested)
	if oldDigested || newDigested {
		return tui.FormatImage(oldRef, tui.Red) + " " +
			tui.Faint.Render("→") + " " +
			tui.FormatImage(newRef, tui.Green)
	}

	// If repos match and both are tagged, show only tag diff.
	oldTagged, oldOk := oldRef.(reference.NamedTagged)
	newTagged, newOk := newRef.(reference.NamedTagged)
	if oldOk && newOk && reference.FamiliarName(oldRef) == reference.FamiliarName(newRef) {
		return reference.FamiliarName(newRef) +
			tui.Faint.Render(":") +
			tui.Red.Render(oldTagged.Tag()) + " " +
			tui.Faint.Render("→") + " " +
			tui.Green.Render(newTagged.Tag())
	}

	// Different repos: full old → new.
	return tui.FormatImage(oldRef, tui.Red) + " " +
		tui.Faint.Render("→") + " " +
		tui.FormatImage(newRef, tui.Green)
}

// NewDeployment creates a new deployment for the given service specification.
// If strategy is nil, a default RollingStrategy will be used.
func NewDeployment(cli Client, spec api.ServiceSpec, strategy Strategy) *Deployment {
	if strategy == nil {
		strategy = &RollingStrategy{}
	}

	return &Deployment{
		Spec:     spec,
		Strategy: strategy,
		cli:      cli,
	}
}

// NewDeploymentWithClusterState creates a new deployment like NewDeployment but also with a provided current cluster
// state used for scheduling decisions.
func NewDeploymentWithClusterState(
	cli Client, spec api.ServiceSpec, strategy Strategy, state *scheduler.ClusterState,
) *Deployment {
	d := NewDeployment(cli, spec, strategy)
	d.state = state
	return d
}

// Plan returns a plan of operations to reconcile the service to the desired state.
// If a plan has already been created, the same plan will be returned.
func (d *Deployment) Plan(ctx context.Context) (ServicePlan, error) {
	if d.plan != nil {
		return *d.plan, nil
	}

	// Validate the user-provided spec before resolving it.
	if err := d.Validate(ctx); err != nil {
		return ServicePlan{}, fmt.Errorf("invalid deployment: %w", err)
	}

	clusterDomain, err := d.cli.GetDomain(ctx)
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		return ServicePlan{}, fmt.Errorf("get cluster domain: %w", err)
	}
	specResolver := &ServiceSpecResolver{
		// If the domain is not found (not reserved), an empty domain is used for the resolver.
		ClusterDomain: clusterDomain,
	}

	resolvedSpec, err := specResolver.Resolve(d.Spec)
	if err != nil {
		return ServicePlan{}, fmt.Errorf("resolve service spec: %w", err)
	}

	if d.state == nil {
		d.state, err = scheduler.InspectClusterState(ctx, d.cli)
		if err != nil {
			return ServicePlan{}, fmt.Errorf("inspect cluster state: %w", err)
		}
	}

	plan, err := d.Strategy.Plan(d.state, d.Service, resolvedSpec)
	if err != nil {
		return ServicePlan{}, fmt.Errorf("create plan using %s strategy: %w", d.Strategy.Type(), err)
	}
	d.plan = &plan

	return plan, nil
}

// Validate checks if the deployment specification is valid.
func (d *Deployment) Validate(ctx context.Context) error {
	if err := d.Spec.Validate(); err != nil {
		return fmt.Errorf("invalid service spec: %w", err)
	}

	if d.Service == nil {
		svc, err := d.cli.InspectService(ctx, d.Spec.Name)
		if err == nil {
			d.Service = &svc
		} else if !errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("inspect service: %w", err)
		}
	}
	// d.Service is nil if the service doesn't exist yet (first deployment).
	if d.Service == nil {
		return nil
	}

	if d.Service.Name != d.Spec.Name {
		return errors.New("service name cannot be changed")
	}

	// Resolve the default mode if not specified.
	mode := d.Spec.Mode
	if mode == "" {
		mode = api.ServiceModeReplicated
	}

	if mode != d.Service.Mode {
		return errors.New("service mode cannot be changed")
	}

	return nil
}

// Run executes the deployment plan and returns the ID of the created or updated service.
// It will create a new plan if one hasn't been created yet. The deployment will either create a new service or update
// the existing one to match the desired specification.
// TODO: forbid to run the same deployment more than once.
func (d *Deployment) Run(ctx context.Context) (ServicePlan, error) {
	plan, err := d.Plan(ctx)
	if err != nil {
		return plan, fmt.Errorf("create plan: %w", err)
	}

	return plan, plan.Execute(ctx, d.cli)
}
