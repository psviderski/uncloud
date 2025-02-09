package fs

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

func ExpandHomeDir(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return strings.Replace(path, "~", home, 1)
	}
	return path
}

// LookupUIDGID returns the user and group IDs for the given username.
func LookupUIDGID(username string) (uid, gid int, err error) {
	usr, err := user.Lookup(username)
	if err != nil {
		err = fmt.Errorf("lookup user %q: %w", username, err)
		return
	}
	uid, err = strconv.Atoi(usr.Uid)
	if err != nil {
		err = fmt.Errorf("parse %q user ID (UID) %q: %w", username, usr.Uid, err)
		return
	}
	gid, err = strconv.Atoi(usr.Gid)
	if err != nil {
		err = fmt.Errorf("parse %q user group ID (GID) %q: %w", username, usr.Gid, err)
		return
	}
	return
}

func Chown(path, username, group string) error {
	uid, gid := -1, -1
	if username != "" {
		usr, err := user.Lookup(username)
		if err != nil {
			return fmt.Errorf("lookup user %q: %w", username, err)
		}
		uid, err = strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("parse %q user ID (UID) %q: %w", username, usr.Uid, err)
		}
	}

	if group != "" {
		grp, err := user.LookupGroup(group)
		if err != nil {
			return fmt.Errorf("lookup group %q: %w", group, err)
		}
		gid, err = strconv.Atoi(grp.Gid)
		if err != nil {
			return fmt.Errorf("parse %q group ID (GID) %q: %w", group, grp.Gid, err)
		}
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %q: %w", path, err)
	}
	return nil
}
