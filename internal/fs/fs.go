package fs

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
)

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

func Chown(path, owner string) error {
	uid, gid, err := LookupUIDGID(owner)
	if err != nil {
		return err
	}
	if err = os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %q: %w", path, err)
	}
	return nil
}
