//go:build darwin

package network

import "errors"

// detectEgressMTU is a stub for Darwin. The machine daemon that performs detection only runs on Linux.
func detectEgressMTU() (int, error) {
	return 0, errors.New("not implemented on darwin")
}
