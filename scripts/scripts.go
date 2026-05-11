// Package scripts embeds shell scripts shipped with the uc CLI so they can be
// executed on remote machines without being fetched from the network.
package scripts

import _ "embed"

//go:embed install.sh
var InstallScript string
