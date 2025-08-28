package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const docsDir = "website/docs/9-cli-reference"

type cmdWrapper struct {
	cmd *cobra.Command
}

// NewDocsCommand creates a new hidden command to generate CLI reference docs.
func NewDocsCommand() *cobra.Command {
	wrapper := &cmdWrapper{}
	cmd := &cobra.Command{
		Use:                   "docs",
		Short:                 "Generate Uncloud CLI reference docs",
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		Hidden:                true,
		Args:                  cobra.NoArgs,
		ValidArgsFunction:     cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Remove existing markdown files.
			mdFiles, err := filepath.Glob(filepath.Join(docsDir, "*.md"))
			if err != nil {
				return fmt.Errorf("list existing CLI docs: %w", err)
			}
			for _, f := range mdFiles {
				if err = os.Remove(f); err != nil {
					return fmt.Errorf("remove '%s': %w", f, err)
				}
			}

			// Generate new CLI reference docs.
			wrapper.cmd.Root().DisableAutoGenTag = true
			if err := doc.GenMarkdownTree(cmd.Root(), docsDir); err != nil {
				return fmt.Errorf("generate CLI docs: %w", err)
			}

			// Remove *completion*.md files that contain malformatted code blocks that break Docusaurus.
			mdFiles, err = filepath.Glob(filepath.Join(docsDir, "*completion*.md"))
			if err != nil {
				return fmt.Errorf("list generated CLI docs: %w", err)
			}
			for _, f := range mdFiles {
				if err = os.Remove(f); err != nil {
					return fmt.Errorf("remove '%s': %w", f, err)
				}
			}

			// Post-process generated markdown files.
			mdFiles, err = filepath.Glob(filepath.Join(docsDir, "*.md"))
			if err != nil {
				return fmt.Errorf("list generated CLI docs: %w", err)
			}

			for _, f := range mdFiles {
				if err = postProcessMarkdown(f); err != nil {
					return fmt.Errorf("post-process '%s': %w", f, err)
				}
			}

			return nil
		},
	}

	wrapper.cmd = cmd
	return cmd
}

// postProcessMarkdown applies transformations to generated markdown files.
func postProcessMarkdown(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	content := string(data)

	// Replace "SEE ALSO" with "See also".
	content = strings.ReplaceAll(content, "SEE ALSO", "See also")
	// Escape <id> to avoid Docusaurus treating it as an HTML tag.
	content = strings.ReplaceAll(content, "<id>", "\\<id>")

	// Remove broken links to completion docs.
	if strings.Contains(content, "[uc completion") {
		lines := strings.Split(content, "\n")
		var filteredLines []string
		for _, line := range lines {
			if !strings.Contains(line, "[uc completion") {
				filteredLines = append(filteredLines, line)
			}
		}
		content = strings.Join(filteredLines, "\n")
	}

	// Adjust heading levels. Process from shortest to longest to avoid double replacements.
	replacements := []struct {
		old, new string
	}{
		{`(?m)^## `, `# `},
		{`(?m)^### `, `## `},
		{`(?m)^#### `, `### `},
		{`(?m)^##### `, `#### `},
	}

	for _, r := range replacements {
		re := regexp.MustCompile(r.old)
		content = re.ReplaceAllString(content, r.new)
	}

	return os.WriteFile(filename, []byte(content), 0o644)
}
