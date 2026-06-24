//go:build !linux

package osinfo

// KernelVersion returns an empty string on non-Linux hosts where the machine daemon does not run.
func KernelVersion() string {
	return ""
}
