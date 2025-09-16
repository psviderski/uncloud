package cli

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ExpandCommaSeparatedValues takes a slice of strings and expands any comma-separated values into individual elements.
func ExpandCommaSeparatedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	var expanded []string
	for _, value := range values {
		for _, v := range strings.Split(value, ",") {
			if v = strings.TrimSpace(v); v != "" {
				expanded = append(expanded, v)
			}
		}
	}

	return expanded
}

// BindEnvToFlag assigns the value of an environment variable to the given command flag if the flag has not been set.
func BindEnvToFlag(cmd *cobra.Command, flagName, envVar string) {
	if value := os.Getenv(envVar); value != "" && !cmd.Flags().Changed(flagName) {
		if err := cmd.Flags().Set(flagName, value); err != nil {
			log.Fatalf("Failed to bind environment variable '%s' to flag '%s': %v", envVar, flagName, err)
		}
	}
}
