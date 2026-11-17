//go:build linux

package network

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func TestCreateOrGetLink(t *testing.T) {
	link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "wg-test"}}
	userspace := &userspaceWireGuard{}
	startErr := errors.New("userspace failed")

	tests := []struct {
		name          string
		existing      bool
		addErr        error
		startErr      error
		wantUserspace bool
		wantErr       bool
		wantAddCalls  int
		wantStartCall int
	}{
		{name: "existing link", existing: true},
		{name: "kernel WireGuard", wantAddCalls: 1},
		{name: "missing kernel module", addErr: unix.EOPNOTSUPP, wantUserspace: true, wantAddCalls: 1, wantStartCall: 1},
		{name: "missing kernel device", addErr: unix.ENODEV, wantUserspace: true, wantAddCalls: 1, wantStartCall: 1},
		{name: "userspace failure", addErr: unix.EOPNOTSUPP, startErr: startErr, wantErr: true, wantAddCalls: 1, wantStartCall: 1},
		{name: "permission error", addErr: unix.EPERM, wantErr: true, wantAddCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookupCalls, addCalls, startCalls := 0, 0, 0
			linkByName := func(string) (netlink.Link, error) {
				lookupCalls++
				if tt.existing || lookupCalls > 1 {
					return link, nil
				}
				return nil, netlink.LinkNotFoundError{}
			}
			linkAdd := func(netlink.Link) error {
				addCalls++
				return tt.addErr
			}
			startUserspace := func(string) (*userspaceWireGuard, error) {
				startCalls++
				return userspace, tt.startErr
			}

			gotLink, gotUserspace, err := createOrGetLinkWith("wg-test", linkByName, linkAdd, startUserspace)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, gotLink)
			} else {
				require.NoError(t, err)
				require.Same(t, link, gotLink)
			}
			if tt.wantUserspace {
				require.Same(t, userspace, gotUserspace)
			} else {
				require.Nil(t, gotUserspace)
			}
			require.Equal(t, tt.wantAddCalls, addCalls)
			require.Equal(t, tt.wantStartCall, startCalls)
		})
	}
}

func TestKernelWireGuardUnavailable(t *testing.T) {
	for _, err := range []error{unix.EOPNOTSUPP, unix.ENODEV, fmt.Errorf("wrapped: %w", unix.EOPNOTSUPP)} {
		require.True(t, kernelWireGuardUnavailable(err))
	}
	for _, err := range []error{unix.EPERM, unix.EEXIST, errors.New("netlink failed")} {
		require.False(t, kernelWireGuardUnavailable(err))
	}
}
