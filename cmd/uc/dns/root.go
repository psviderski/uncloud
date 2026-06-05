package dns

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns",
		Short: "Manage cluster domain in Uncloud DNS.",
		Long: "Manage cluster domain in Uncloud DNS.\n" +
			"DNS commands allow you to reserve or release a unique 'xxxxxx.uncld.dev' domain for your " +
			"cluster. When reserved, Caddy service deployments will automatically update DNS records to route " +
			"traffic to the services in the cluster.",
	}
	cmd.AddCommand(
		NewReleaseCommand(),
		NewReserveCommand(),
		NewShowCommand(),
	)
	return cmd
}
