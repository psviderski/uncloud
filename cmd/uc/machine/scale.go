package machine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/cmd/uc/caddy"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/deploy"
)

// scale inspects all services and scales global ones to use the extra machine.
func scale(ctx context.Context, clusterClient, machineClient *client.Client, uncli *cli.CLI, yes bool) error {
	services, err := globalServices(ctx, clusterClient)
	if err != nil {
		return err
	}

	var (
		d    *deploy.Deployment
		plan deploy.ServicePlan
	)
	for _, service := range services {
		switch service.Name {
		case client.CaddyServiceName:
			// NOTE: We use the cluster client to inspect and scale the Caddy service because the newly added machine may have
			// issues accessing the Machine API of existing machines in the cluster.
			// See the issue for more details: https://github.com/psviderski/uncloud/issues/65.
			caddyImage := service.Containers[0].Container.Config.Image
			// Find the latest created container and use its image.
			var latestCreated time.Time
			for _, c := range service.Containers[1:] {
				created, err := time.Parse(time.RFC3339Nano, c.Container.Created)
				if err != nil {
					continue
				}
				if created.After(latestCreated) {
					latestCreated = created
					caddyImage = c.Container.Config.Image
				}
			}
			d, err := clusterClient.NewCaddyDeployment(caddyImage, "", api.Placement{})
			if err != nil {
				return fmt.Errorf("create %s deployment: %w", service.Name, err)
			}
			if plan, err = d.Plan(ctx); err != nil {
				return fmt.Errorf("plan %s deployment: %w", service.Name, err)
			}

		default:
			spec := service.Containers[0].Container.ServiceSpec
			d = clusterClient.NewDeployment(spec, nil)
			if plan, err = d.Plan(ctx); err != nil {
				return fmt.Errorf("plan %s deployment: %w", service.Name, err)
			}
		}

		if len(plan.Operations) == 0 {
			fmt.Printf("Service %s is up to date.\n", service.Name)
			continue
		}

		if !yes {
			confirmed, err := tui.Confirm("Proceed with deployment?")
			if err != nil {
				return fmt.Errorf("confirm deployment: %w", err)
			}
			if !confirmed {
				fmt.Printf("Scaling %s cancelled.", service.Name)
				continue
			}
		}

		fmt.Println(tui.Bold.Underline(true).Render("Scaling plan"))
		fmt.Println()
		fmt.Print(plan.Format())

		summary := plan.FormatSummary()
		fmt.Println(tui.Faint.Render(strings.Repeat("─", lipgloss.Width(summary))))
		fmt.Println(summary)
		fmt.Println()

		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if _, err = d.Run(ctx); err != nil {
				return fmt.Errorf("scale %s: %w", service.Name, err)
			}
			return nil
		}, uncli.ProgressOut(), fmt.Sprintf("Scaling service %s (%s mode)", d.Spec.Name, d.Spec.Mode))
		if err != nil {
			return err
		}
		if service.Name == client.CaddyServiceName {
			fmt.Println()
			if err := caddy.UpdateDomainRecords(ctx, machineClient, uncli.ProgressOut()); err != nil {
				return err
			}
		}
	}

	return nil
}

func globalServices(ctx context.Context, clusterClient *client.Client) ([]api.Service, error) {
	services, err := clusterClient.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	return slices.DeleteFunc(services, func(s api.Service) bool {
		return s.Mode != api.ServiceModeGlobal
	}), nil
}
