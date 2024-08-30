package main

import (
	"context"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"uncloud/internal/machine"
	"uncloud/internal/machine/daemon"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	var dataDir string
	cmd := &cobra.Command{
		Use:           "uncloudd",
		Short:         "Uncloud machine daemon.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Run(cmd.Context(), dataDir)
		},
	}
	cmd.PersistentFlags().StringVarP(&dataDir, "data-dir", "d", machine.DefaultDataDir,
		"Directory for storing persistent machine state")
	_ = cmd.MarkFlagDirname("data-dir")

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
	slog.Info("Daemon stopped.")
}
