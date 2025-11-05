package log

import (
	"log/slog"
	"os"
	"slices"
	"strings"
)

func InitLoggerFromEnv() {
	debugValues := []string{"1", "true", "yes"}
	if slices.Contains(debugValues, strings.ToLower(os.Getenv("DEBUG"))) {
		logger := slog.New(NewSlogTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)
	}
	slog.Debug("logger initialized")
}
