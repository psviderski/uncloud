package cli

import (
	"fmt"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"net/netip"
	"uncloud/internal/machine/network"
	"uncloud/internal/secret"
)

type User struct {
	privateKey wgtypes.Key
}

func NewUser(privateKey secret.Secret) (*User, error) {
	var (
		wgKey wgtypes.Key
		err   error
	)
	if privateKey == nil {
		wgKey, err = wgtypes.GeneratePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("generate key for user: %w", err)
		}
		privateKey = wgKey[:]
	} else {
		wgKey, err = wgtypes.NewKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("invalid key: %w", err)
		}
		privateKey = wgKey[:]
	}
	return &User{
		privateKey: wgKey,
	}, nil
}

func (u *User) PrivateKey() secret.Secret {
	return u.privateKey[:]
}

func (u *User) PublicKey() secret.Secret {
	pubKey := u.privateKey.PublicKey()
	return pubKey[:]
}

func (u *User) IPv6() netip.Addr {
	return network.PeerIPv6(u.PublicKey())
}
