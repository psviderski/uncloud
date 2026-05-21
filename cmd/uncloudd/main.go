package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/daemon"
	"github.com/psviderski/uncloud/internal/log"
	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	logger := slog.New(log.NewSlogTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	var dataDir string
	cmd := &cobra.Command{
		Use:           "uncloudd",
		Short:         "Uncloud machine daemon.",
		Long:          "Uncloud machine daemon.\n" + tui.URLStyle.Render(version.WebsiteURL),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := daemon.New(dataDir)
			if err != nil {
				return err
			}
			if err = d.Run(cmd.Context()); err == nil {
				slog.Info("Daemon stopped.")
			}
			return err
		},
	}
	cmd.PersistentFlags().StringVarP(&dataDir, "data-dir", "d", machine.DefaultDataDir,
		"Directory for storing persistent machine state.")
	_ = cmd.MarkFlagDirname("data-dir")

	cmd.AddCommand(newDialStdioCommand())
	cmd.AddCommand(newVersionCommand())

	// ctx is canceled when the daemon command is interrupted.
	ctx, cancel := context.WithCancel(context.Background())

	// Handle interrupt signals and cancel the context.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		slog.Info("Received signal, stopping daemon.", "signal", sig)
		cancel()
	}()

	cobra.CheckErr(cmd.ExecuteContext(ctx))
}
