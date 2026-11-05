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
	"github.com/psviderski/uncloud/api/pb"
	"github.com/psviderski/uncloud/internal/cli"
	"github.com/psviderski/uncloud/internal/cli/completion"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/spf13/cobra"
)

type deployOptions struct {
	caddyfile string
	image     string
	machines  []string

	caddyfileChanged bool
	imageChanged     bool
	machinesChanged  bool
}

func NewDeployCommand() *cobra.Command {
	opts := deployOptions{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy or upgrade Caddy reverse proxy across all machines in the cluster.",
		Long: `Deploy or upgrade Caddy reverse proxy across all machines in the cluster.
A rolling update is performed when updating existing containers to minimise disruption.

By default, this deploys the latest version of the Uncloud Caddy image with a global config that enables cluster
storage for TLS certificates. See https://github.com/unlabs-dev/caddy-uncloud for more details.

Values set with --image, --caddyfile, and --machine are kept for later deployments. Pass an empty value to reset
a flag to its default.

If this command didn't deploy the current caddy service, for example, it was deployed with 'uc deploy'
from a Compose file or an older version of uc, the deployment replaces its configuration.

Requires Uncloud daemon ` + client.MinCaddyDaemonVersion + ` or newer on all machines.`,
		Example: `  # Deploy Caddy or upgrade it to the latest version of the Uncloud Caddy image.
  uc caddy deploy

  # Deploy a custom image and keep it for later deployments.
  uc caddy deploy --image ghcr.io/unlabs-dev/caddy-uncloud:0.1.3

  # Go back to the latest version of the Uncloud Caddy image.
  uc caddy deploy --image ""

  # Deploy a custom image build and a custom global Caddy config and keep both for later deployments.
  uc caddy deploy --image myorg/caddy-uncloud:latest --caddyfile global.Caddyfile

  # Go back to the default global Caddy config but keep the custom image.
  uc caddy deploy --caddyfile ""

  # Deploy only to specific machines and keep them for later deployments.
  uc caddy deploy --machine machine1,machine2

  # Go back to deploying to all machines.
  uc caddy deploy --machine ""`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uncli := cmd.Context().Value("cli").(*cli.CLI)

			opts.caddyfileChanged = cmd.Flag("caddyfile").Changed
			opts.imageChanged = cmd.Flag("image").Changed
			opts.machinesChanged = cmd.Flag("machine").Changed

			return runDeploy(cmd.Context(), uncli, opts)
		},
	}

	cmd.Flags().StringVar(&opts.caddyfile, "caddyfile", "",
		"Path to a custom global Caddy config (Caddyfile) that will be prepended to the auto-generated Caddy config.\n"+
			"An empty value resets it to the default config that enables cluster storage.\n"+
			"(default is the current custom config or the default config)")
	cmd.Flags().StringVar(&opts.image, "image", "",
		"Caddy Docker image to deploy. It must include the Uncloud storage module if the Caddy config uses it.\n"+
			"An empty value resets it to the latest version of the Uncloud Caddy image.\n"+
			"(default is the current custom image or "+client.CaddyImage+":LATEST_VERSION)")
	cmd.Flags().StringSliceVarP(&opts.machines, "machine", "m", nil,
		"Machine names or IDs to deploy to. Can be specified multiple times or as a comma-separated list.\n"+
			"An empty value resets it to all machines.\n"+
			"(default is the current machines or all machines)")

	completion.MachinesFlag(cmd)

	return cmd
}

func runDeploy(ctx context.Context, uncli *cli.CLI, opts deployOptions) error {
	config := ""
	if opts.caddyfile != "" {
		data, err := os.ReadFile(opts.caddyfile)
		if err != nil {
			return fmt.Errorf("read Caddyfile: %w", err)
		}
		config = strings.TrimSpace(string(data))
	}

	clusterClient, err := uncli.ConnectCluster(ctx)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}
	defer clusterClient.Close()

	if err = clusterClient.Caddy.CheckDaemonVersions(ctx); err != nil {
		return err
	}

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

	current := svc.Spec()
	deployOpts := resolveDeploymentOptions(current, opts, config)
	configChanged := deployOpts.ConfigOrDefault() != current.CaddyConfig()

	if svc.ID != "" && configChanged {
		printCaddyConfig("current global Caddy config:", current.CaddyConfig())
	}

	fmt.Println()
	fmt.Println("Preparing a deployment plan...")

	d, err := clusterClient.Caddy.NewDeployment(ctx, deployOpts)
	if err != nil {
		return fmt.Errorf("create caddy deployment: %w", err)
	}
	target := d.Spec

	// The Uncloud Caddy image resolves to the latest version allowed by this CLI, which is older than the running image
	// if a newer CLI deployed it. Continuing would downgrade the image and replace its default config.
	if deployOpts.Image == "" && client.IsNewerCaddyImage(current.Container.Image, target.Container.Image) {
		return fmt.Errorf("caddy service runs image '%s' which is newer than the latest version '%s' supported by "+
			"this uc version: upgrade uc or specify the image with --image",
			current.Container.Image, target.Container.Image)
	}

	imageNote := " (latest)"
	if deployOpts.Image != "" {
		imageNote = " (custom)"
		if !opts.imageChanged {
			imageNote = " (custom, use --image \"\" for the latest version)"
		}
	}
	fmt.Println(tui.Faint.Render("target image: ") + tui.FormatImage(target.Container.Image, tui.Green) +
		tui.Faint.Render(imageNote))

	if configChanged {
		if deployOpts.Config != nil {
			// The user provided this config so there is no need to print its content.
			fmt.Println(tui.Faint.Render("target global Caddy config: ") + opts.caddyfile)
		} else {
			printCaddyConfig("target global Caddy config: (default)", deployOpts.ConfigOrDefault())
		}
	}

	plan, err := d.Plan(ctx)
	if err != nil {
		return fmt.Errorf("plan caddy deployment: %w", err)
	}

	if len(plan.Operations) == 0 {
		fmt.Printf("%s service is up to date.\n", client.CaddyServiceName)
	} else {
		if svc.ID == "" {
			if len(deployOpts.Placement.Machines) > 0 {
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

		warnings := deploymentWarnings(current, target, opts)
		for _, warning := range warnings {
			tui.PrintWarning(warning)
		}
		if len(warnings) > 0 {
			fmt.Println()
		}

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

// resolveDeploymentOptions returns the deployment options for the flags and the current service spec, which is zero if
// the service doesn't exist. Each flag works on its own: a specified flag overrides the value and an empty one resets
// it to the default. A flag that isn't specified keeps the explicitly set value.
func resolveDeploymentOptions(
	current api.ServiceSpec, opts deployOptions, config string,
) client.CaddyDeploymentOptions {
	var resolved client.CaddyDeploymentOptions

	if opts.imageChanged {
		resolved.Image = opts.image
	} else if _, custom := current.Labels[api.LabelCaddyImageCustom]; custom {
		resolved.Image = current.Container.Image
	}

	if opts.caddyfileChanged {
		if opts.caddyfile != "" {
			resolved.Config = new(config)
		}
	} else if _, custom := current.Labels[api.LabelCaddyConfigCustom]; custom {
		resolved.Config = new(current.CaddyConfig())
	}

	if opts.machinesChanged {
		resolved.Placement.Machines = cli.ExpandCommaSeparatedValues(opts.machines)
	} else if _, managed := current.Labels[api.LabelCaddyManaged]; managed {
		resolved.Placement = current.Placement
	}

	return resolved
}

// deploymentWarnings returns the consequences of deploying the target spec over the current one, which is zero if
// the service doesn't exist.
func deploymentWarnings(current, target api.ServiceSpec, opts deployOptions) []string {
	var warnings []string
	exists := current.Name != ""

	if _, managed := current.Labels[api.LabelCaddyManaged]; exists && !managed {
		warnings = append(warnings, "'uc caddy deploy' doesn't manage this caddy service yet, so this deployment "+
			"replaces its configuration, including any custom global Caddy config.\n"+
			"If you manage it with a Compose file, cancel and deploy with 'uc deploy' instead.")
	}

	currentStorage := client.CaddyStorageModule(current.CaddyConfig())
	targetStorage := client.CaddyStorageModule(target.CaddyConfig())
	if exists && currentStorage != targetStorage {
		warning := fmt.Sprintf("Caddy certificate storage changes from %s to %s.\n"+
			"Caddy doesn't automatically migrate certificates, so all of them will be reissued and HTTPS may fail "+
			"for those domains until that finishes.\n"+
			"To migrate them first, see %s/concepts/ingress/cluster-storage-for-caddy",
			caddyStorageName(currentStorage), caddyStorageName(targetStorage), version.DocsURL)
		if !opts.caddyfileChanged {
			warning += "\nTo keep the current storage, specify a custom Caddyfile that uses it with --caddyfile."
		}
		warnings = append(warnings, warning)
	}

	imageOrConfigChanged := current.Container.Image != target.Container.Image ||
		current.CaddyConfig() != target.CaddyConfig()
	if targetStorage == client.CaddyStorageUncloud && imageOrConfigChanged &&
		!client.IsDefaultCaddyImage(target.Container.Image) {
		warnings = append(warnings, fmt.Sprintf("Image '%s' must include the Uncloud storage module to use "+
			"'storage uncloud'. Otherwise, Caddy won't start or apply the new config.\n"+
			"Check 'uc caddy config' after the deployment for errors.", target.Container.Image))
	}

	return warnings
}

// caddyStorageName returns a user-friendly name of the Caddy storage module.
func caddyStorageName(module string) string {
	switch module {
	case client.CaddyStorageFileSystem:
		return "local"
	case client.CaddyStorageUncloud:
		return "cluster"
	default:
		return "'" + module + "'"
	}
}

// printCaddyConfig prints the title and the indented config or "none" if it's empty.
func printCaddyConfig(title, config string) {
	if config == "" {
		lipgloss.Println(tui.Faint.Render(title) + " none")
		return
	}

	lipgloss.Println(tui.Faint.Render(title))
	fmt.Println(config)
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
	if !domain.Reserved {
		fmt.Printf("Skipping DNS records update as cluster domain '%s' is managed externally.\n", domain.Name)
		return nil
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
				"machines running caddy containers.\n", domain.Name)
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
