package caddy

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type deployOptions struct {
	caddyfile string
	image     string
	machines  []string
}

func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.",
		Long: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.\n" +
			"A rolling update is performed when updating existing containers to minimise disruption.",
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)
			return runDeploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVar(&opts.caddyfile, "caddyfile", "",
		"Path to a custom global Caddy config (Caddyfile) that will be prepended to the auto-generated Caddy config.")
	cmd.Flags().StringVar(&opts.image, "image", "",
		"Caddy Docker image to deploy. (default caddy:LATEST_VERSION)")
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names or IDs to deploy to. Can be specified multiple times or as a comma-separated "+
			"list. (default is all machines)")

	completion.MachinesFlag(cmd)

	return cmd
}

func runDeploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	caddyfile := ""
	if opts.caddyfile != "" {
		data, err := os.ReadFile(opts.caddyfile)
		if err != nil {
			return fmt.Errorf("read Caddyfile: %w", err)
		}
		caddyfile = strings.TrimSpace(string(data))
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	svc, err := clusterClient.InspectService(ctx, client.CaddyServiceName)
	if err != nil {
		if !errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("inspect caddy service: %w", err)
		}
		fmt.Println(tui.Faint.Render("service: ") + tui.NameStyle.Render(client.CaddyServiceName) +
			tui.Faint.Render(" (not running)"))
	} else {
		fmt.Println(tui.Faint.Render("service: ") + tui.NameStyle.Render(svc.Name) +
			tui.Faint.Render(" ("+svc.Mode+" mode)"))

		// Collect unique images of all containers in the running caddy service.
		images := make(map[string]struct{}, len(svc.Containers))
		for _, c := range svc.Containers {
			images[c.Container.Config.Image] = struct{}{}
		}
		currentImages := slices.Collect(maps.Keys(images))

		if len(currentImages) > 1 {
			formattedImages := make([]string, len(currentImages))
			for i, img := range currentImages {
				formattedImages[i] = tui.FormatImage(img, tui.NoStyle)
			}
			fmt.Println(tui.Faint.Render("current images (multiple versions detected): ") +
				strings.Join(formattedImages, tui.Faint.Render(", ")))
		} else {
			fmt.Println(tui.Faint.Render("current image: ") + tui.FormatImage(currentImages[0], tui.NoStyle))
		}
	}

	if opts.image != "" {
		fmt.Println(tui.Faint.Render("target image: ") + tui.FormatImage(opts.image, tui.Green))
	}

	fmt.Println()
	fmt.Println("Preparing a deployment plan...")

	placement := api.Placement{
		Machines: cli.ExpandCommaSeparatedValues(opts.machines),
	}
	d, err := clusterClient.NewCaddyDeployment(opts.image, caddyfile, placement)
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}

	if opts.image == "" {
		fmt.Println(tui.Faint.Render("target image: ") + tui.FormatImage(d.Spec.Container.Image,
			tui.Green) + tui.Faint.Render(" (latest stable)"))
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Printf("%s service is up to date.\n", client.CaddyServiceName)
	} else {
		if svc.ID == "" {
			if len(opts.machines) > 0 {
				fmt.Println("This will run a Caddy container on each selected machine.")
			} else {
				fmt.Println("This will run a Caddy container on each machine.")
			}
		}

		fmt.Println()
		fmt.Println(tui.Bold.Underline(true).Render("Deployment plan"))
		fmt.Println()

		directConn := uncli.DirectConnection()
		contextName := uncli.ContextOverrideOrCurrent()
		deployTarget := ""
		if directConn != "" {
			deployTarget = directConn
			fmt.Println(tui.Faint.Render("connection: ") + tui.NameStyle.Render(directConn))
			fmt.Println()
		} else if contextName != "" && len(uncli.Config.Contexts) > 1 {
			// Only show context if there's more than one to avoid unnecessary clutter.
			deployTarget = contextName
			fmt.Println(tui.Faint.Render("context: ") + tui.NameStyle.Render(contextName))
			fmt.Println()
		}

		fmt.Println(plan.Format())

		summary := plan.FormatSummary()
		fmt.Println(tui.Faint.Render(strings.Repeat("─", lipgloss.Width(summary))))
		fmt.Println(summary)
		fmt.Println()

		title := "Proceed with deployment?"
		// Include the direct connection or context name in the confirmation prompt to avoid accidentally
		// deploying to the wrong cluster.
		if deployTarget != "" {
			isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
			confirmStyle := tui.ThemeConfirm().Theme(isDark).Focused.Title
			title = "Proceed with deployment to " + tui.NameStyle.Render(deployTarget) + confirmStyle.Render("?")
		}

		confirmed, err := tui.Confirm(title)
		if err != nil {
			return fmt.Errorf("confirm deployment: %w", err)
		}
		if !confirmed {
			return cli.Cancelled("Caddy deploy cancelled. No changes were made.")
		}

		err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
			if _, err = d.Run(ctx); err != nil {
				return fmt.Errorf("deploy caddy: %w", err)
			}
			return nil
		}, uncli.ProgressOut(), fmt.Sprintf("Deploying service %s (%s mode)", d.Spec.Name, d.Spec.Mode))
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return UpdateDomainRecords(ctx, clusterClient, uncli.ProgressOut())
}

func UpdateDomainRecords(ctx context.Context, clusterClient *client.Client, progressOut *streams.Out) error {
	domain, err := clusterClient.GetDomain(ctx)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			fmt.Println("Skipping DNS records update as no cluster domain is reserved (see 'uc dns').")
			return nil
		}
		return fmt.Errorf("get cluster domain: %w", err)
	}

	fmt.Println("Updating cluster domain records in Uncloud DNS to point to machines running caddy service...")
	// TODO: split the method into two: one to get the records and one to update them to ask for update confirmation.

	var records []*pb.DNSRecord
	err = progress.RunWithTitle(ctx, func(ctx context.Context) error {
		var err error
		records, err = clusterClient.CreateIngressRecords(ctx, client.CaddyServiceName)
		return err
	}, progressOut, "Verifying internet access to caddy service")
	if err != nil {
		if errors.Is(err, client.ErrNoReachableMachines) {
			fmt.Println()
			fmt.Printf("DNS records for domain '%s' could not be updated as there are no internet-reachable "+
				"machines running caddy containers.\n", domain)
			fmt.Println()
			fmt.Println("Possible solutions:")
			fmt.Println("- Ensure your machines have public IP addresses")
			fmt.Println("- Use --public-ip flag when adding machines to override the automatically detected IPs")
			fmt.Println("- Check firewall settings on your machines")
			fmt.Println("- Configure port forwarding if behind NAT")
			fmt.Println("- Retry Caddy deployment with 'uc caddy deploy' after resolving connectivity issues")
			fmt.Println()
			fmt.Println("Your services won't be accessible from the internet until at least one machine " +
				"becomes reachable. If you aren't planning to expose any services publicly, you can release " +
				"the domain by running 'uc dns release'.")
		}
		return fmt.Errorf("failed to update DNS records pointing to caddy service: %w", err)
	}

	fmt.Println()
	fmt.Println("DNS records updated to use only the internet-reachable machines running caddy service:")
	for _, r := range records {
		fmt.Printf("  %s  %s → %s\n", r.Name, r.Type, strings.Join(r.Values, ", "))
	}

	return nil
}
