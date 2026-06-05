package main

import (
	"strings"

	"github.com/psviderski/uncloud/cmd/uncloud/image"
	"github.com/spf13/cobra"
)

// NewImagesCommand returns the 'image ls' command modified to work as 'images'.
func NewImagesCommand() *cobra.Command {
	listCmd := image.NewListCommand()
	listCmd.Use = "images [IMAGE]"
	// Remove 'list' alias since this command is already an alias.
	listCmd.Aliases = nil
	listCmd.Example = strings.ReplaceAll(listCmd.Example, "uc image ls", "uc images")

	return listCmd
}
