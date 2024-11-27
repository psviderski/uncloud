package fs

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

func MkDataDir(dir, owner string) error {
	parent, _ := filepath.Split(dir)
	// Use 0711 for parent directories to allow `owner` to access its nested data directory.
	if err := os.MkdirAll(parent, 0711); err != nil {
		return fmt.Errorf("create directory %q: %w", parent, err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}
	if err := Chown(dir, owner); err != nil {
		return err
	}
	return nil
}

func Chown(path, owner string) error {
	if owner != "" {
		usr, err := user.Lookup(owner)
		if err != nil {
			return fmt.Errorf("lookup user %q: %w", owner, err)
		}
		uid, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("parse %q user ID (UID) %q: %w", owner, usr.Uid, err)
		}
		gid, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return fmt.Errorf("parse %q user group ID (GID) %q: %w", owner, usr.Gid, err)
		}
		if err = os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %q: %w", path, err)
		}
	}
	return nil
}
