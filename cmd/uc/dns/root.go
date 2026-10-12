package dns

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns",
		Short: "Manage the cluster domain.",
		Long: "Manage the cluster domain.\n" +
			"DNS commands allow you to reserve or release a unique 'xxxxxx.uncld.dev' domain for your " +
			"cluster. When reserved, Caddy service deployments will automatically update DNS records to route " +
			"traffic to the services in the cluster.\n\n" +
			"EXPERIMENTAL: Use 'uc dns set' to configure an externally managed domain instead. " +
			"Uncloud does not manage DNS records for manually set domains.",
	}
	cmd.AddCommand(
		NewReleaseCommand(),
		NewReserveCommand(),
		NewShowCommand(),
		NewSetCommand(),
	)
	return cmd
}
