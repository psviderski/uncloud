package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/internal/version"
	"github.com/spf13/cobra"
)

//go:embed art.txt
var asciiArt string

// NewVersionCommand creates a new command to print the version and build information for the binary.
func NewVersionCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version and build information.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.GetInfo()
			w := cmd.OutOrStdout()

			switch output {
			case "":
				fmt.Fprint(w, humanVersion(info))
			case "json":
				s, err := info.JSONString()
				if err != nil {
					return err
				}
				fmt.Fprintln(w, s)
			default:
				s, err := templateVersion(output, info)
				if err != nil {
					return err
				}
				fmt.Fprint(w, s)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "",
		"Output format: 'json' or a Go template (e.g. '{{.Version}}').\n"+
			"Run with '-o json' to discover the field names available to the template.\n"+
			"(default is human-readable)")

	return cmd
}

func humanVersion(info version.Info) string {
	var b strings.Builder
	b.WriteString(asciiArt)
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("uc: Uncloud CLI tool for deploying apps and managing resources (%s)",
		tui.URLStyle.Render(version.WebsiteURL)))
	b.WriteString("\n\n")
	b.WriteString(info.String())
	return b.String()
}

// templateVersion renders the version info using the provided Go template.
func templateVersion(tmpl string, info version.Info) (string, error) {
	t, err := template.New("version").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err = t.Execute(&buf, info); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
