package osinfo

import "golang.org/x/sys/unix"

// KernelVersion returns the running Linux kernel release, e.g. "6.8.0-31-generic".
func KernelVersion() string {
	var un unix.Utsname
	if err := unix.Uname(&un); err != nil {
		return ""
	}
	return unix.ByteSliceToString(un.Release[:])
}
