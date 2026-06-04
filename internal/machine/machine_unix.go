//go:build !windows

package machine

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/docker/go-connections/sockets"
)

// listenUnixSocket creates a new Unix socket listener with the specified path. The socket file is created with 0660
// access mode and uncloud group if the group is found, otherwise it falls back to the root group.
func listenUnixSocket(path string) (net.Listener, error) {
	gid := 0 // Fall back to the root group if the uncloud group is not found.
	group, err := user.LookupGroup(DefaultSockGroup)
	if err != nil {
		//goland:noinspection GoTypeAssertionOnErrors
		if _, ok := err.(user.UnknownGroupError); ok {
			slog.Info(
				"Specified group not found, using root group for the API socket.",
				"group", DefaultSockGroup, "path", path,
			)
		} else {
			return nil, fmt.Errorf("lookup %q group ID (GID): %w", DefaultSockGroup, err)
		}
	} else {
		gid, err = strconv.Atoi(group.Gid)
		if err != nil {
			return nil, fmt.Errorf("parse %q group ID (GID) %q: %w", DefaultSockGroup, group.Gid, err)
		}
	}

	// Ensure the parent directory exists and has the correct group permissions.
	parent, _ := filepath.Split(path)
	if err = os.MkdirAll(parent, 0o750); err != nil {
		return nil, fmt.Errorf("create directory %q: %w", parent, err)
	}
	if err = os.Chown(parent, -1, gid); err != nil {
		return nil, fmt.Errorf("chown directory %q: %w", parent, err)
	}

	return sockets.NewUnixSocket(path, gid)
}
