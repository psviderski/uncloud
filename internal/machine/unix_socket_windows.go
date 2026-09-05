package machine

import (
	"fmt"
	"net"
)

// listenUnixSocket is not supported on Windows.
func listenUnixSocket(path string) (net.Listener, error) {
	return nil, fmt.Errorf("not supported on Windows")
}
