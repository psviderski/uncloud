package machine

import (
	"fmt"
	"net"
)

// listenUnixSocket is a stub for Windows. The Uncloud daemon does not run on
// Windows, so this is only present to allow the CLI packages to compile.
func listenUnixSocket(path string) (net.Listener, error) {
	return nil, fmt.Errorf("unix socket listener not supported on Windows")
}
