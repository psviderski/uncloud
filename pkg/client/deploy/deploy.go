package deploy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client/deploy/scheduler"
	"github.com/psviderski/uncloud/pkg/client/operation"
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
	// IsNewService indicates this plan creates a new service (first deployment) rather than updating an existing one.
	IsNewService bool
	// Spec is the desired service spec being deployed.
	Spec api.ServiceSpec
	operation.SequenceOperation
}

// Format renders the service plan as a styled block with a spec diff and nested container operations.
func (sp *ServicePlan) Format() string {
	// Determine service-level operation type and extract the old spec from container operations.
	// Containers may have different specs after a failed deployment. The first replaced container determines the old
	// spec for the diff because a container updated in place already has the new image. Otherwise, fallback to the
	// first updated container and then the first removed one.
	var hasRun, hasRemove bool
	var replacedSpec, updatedSpec, removedSpec *api.ServiceSpec
	for _, op := range sp.Operations {
		switch o := op.(type) {
		case *operation.RunContainerOperation:
			hasRun = true
		case *operation.ReplaceContainerOperation:
			if replacedSpec == nil {
				replacedSpec = &o.OldContainer.ServiceSpec
			}
		case *operation.UpdateContainerOperation:
			if updatedSpec == nil {
				updatedSpec = &o.Container.ServiceSpec
			}
		case *operation.RemoveContainerOperation:
			hasRemove = true
			if removedSpec == nil {
				removedSpec = &o.Container.ServiceSpec
			}
		}
	}
	oldSpec := cmp.Or(replacedSpec, updatedSpec, removedSpec)

	// Service line modifier and verb.
	var modifier, verb string
	switch {
	case sp.IsNewService:
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
	// TODO: print diff for all changed attributes, not just image, replicas, and labels.
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
		if sp.IsNewService {
			specTable.Row("", "image:", tui.FormatImage(sp.Spec.Container.Image, tui.Green))
		} else {
			specTable.Row("", "image:", tui.FormatImage(sp.Spec.Container.Image, tui.NoStyle))
		}
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
		if sp.IsNewService {
			if sp.Spec.Replicas > 1 {
				specTable.Row("", "replicas:", tui.Green.Render(replicasStr))
			}
		} else if sp.Spec.Replicas > 1 || hasRun || hasRemove {
			mod := ""
			if hasRun || hasRemove {
				mod = "~"
				replicasStr = tui.Green.Render(replicasStr)
			}
			specTable.Row(mod, "replicas:", replicasStr)
		}
	}

	specDiff := specTable.String() + "\n"

	// Render labels as nested blocks outside the table to not widen its attribute column. An existing service without
	// an old spec only has new containers added to up-to-date ones so its labels didn't change.
	if sp.IsNewService || oldSpec != nil {
		var oldLabels, oldContainerLabels map[string]string
		if oldSpec != nil {
			oldLabels, oldContainerLabels = oldSpec.Labels, oldSpec.Container.Labels
		}
		specDiff += formatLabelsDiff("labels:", oldLabels, sp.Spec.Labels, sp.IsNewService)
		specDiff += formatLabelsDiff("container labels:", oldContainerLabels, sp.Spec.Container.Labels, sp.IsNewService)
	}

	treePrefix := tui.Faint.Render("  │ ")
	for line := range strings.Lines(specDiff) {
		out.WriteString(treePrefix + line)
	}

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
	var createCount, startFirstCount, stopFirstCount, updateCount, removeCount int
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
		case *operation.UpdateContainerOperation:
			machines[o.MachineID] = struct{}{}
			updateCount++
		case *operation.RemoveContainerOperation:
			machines[o.MachineID] = struct{}{}
			removeCount++
		case *operation.StopContainerOperation:
			// Stop operations aren't counted because a later operation in the plan removes the stopped container.
			machines[o.MachineID] = struct{}{}
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
	if updateCount > 0 {
		parts = append(parts,
			tui.BoldYellow.Render(strconv.Itoa(updateCount))+" "+tui.Yellow.Render("update (in place)"))
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
	if oldImage == "" {
		return tui.FormatImage(newImage, tui.Green)
	}

	if oldImage == newImage {
		return tui.FormatImage(newImage, tui.NoStyle)
	}

	oldRef, _ := reference.ParseDockerRef(oldImage)
	newRef, _ := reference.ParseDockerRef(newImage)

	// If either uses a digest, show full old → new.
	_, oldDigested := oldRef.(reference.Digested)
	_, newDigested := newRef.(reference.Digested)
	if oldDigested || newDigested {
		return tui.FormatImage(oldImage, tui.Red) + " " +
			tui.Faint.Render("→") + " " +
			tui.FormatImage(newImage, tui.Green)
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
	return tui.FormatImage(oldImage, tui.Red) + " " +
		tui.Faint.Render("→") + " " +
		tui.FormatImage(newImage, tui.Green)
}

// formatLabelsDiff renders a block with the attribute name followed by a nested line for each added, changed, or
// removed label, sorted by key. It returns an empty string if the labels are the same.
func formatLabelsDiff(attribute string, oldLabels, newLabels map[string]string, isNewService bool) string {
	keys := slices.Concat(slices.Collect(maps.Keys(oldLabels)), slices.Collect(maps.Keys(newLabels)))
	slices.Sort(keys)

	var lines strings.Builder
	for _, key := range slices.Compact(keys) {
		oldValue, inOld := oldLabels[key]
		newValue, inNew := newLabels[key]

		var mod, diff string
		switch {
		case !inOld:
			mod = tui.Green.Render("+")
			// All attributes of a new service are new so only the green color marks them, without a modifier.
			if isNewService {
				mod = " "
			}
			diff = tui.Green.Render(key+tui.Faint.Render("=")) + tui.Green.Render(formatLabelValue(newValue))
		case !inNew:
			mod = tui.Red.Render("-")
			diff = tui.Red.Render(key+tui.Faint.Render("=")) + tui.Red.Render(formatLabelValue(oldValue))
		case oldValue != newValue:
			mod = tui.Yellow.Render("~")
			diff = key +
				tui.Faint.Render("=") +
				tui.Red.Render(formatLabelValue(oldValue)) + " " +
				tui.Faint.Render("→") + " " +
				tui.Green.Render(formatLabelValue(newValue))
		default:
			continue
		}

		lines.WriteString("    " + mod + " " + diff + "\n")
	}
	if lines.Len() == 0 {
		return ""
	}

	blockMod := tui.Yellow.Render("~")
	switch {
	case isNewService:
		blockMod = " "
	case len(oldLabels) == 0:
		blockMod = tui.Green.Render("+")
	case len(newLabels) == 0:
		blockMod = tui.Red.Render("-")
	}

	return blockMod + " " + tui.Faint.Render(attribute) + "\n" + lines.String()
}

// formatLabelValue returns the label value on a single line. It quotes and escapes empty values and values with
// non-printable characters such as newlines so that they're visible in the diff. Other values are returned as is.
func formatLabelValue(value string) string {
	if value == "" || strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return strconv.Quote(value)
	}
	return value
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
		// If no domain is configured, an empty domain is used for the resolver.
		ClusterDomain: clusterDomain.Name,
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

	if d.Service == nil && d.Spec.Name != "" {
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
