package client

import (
	"fmt"
	"github.com/psviderski/uncloud/internal/machine/network"
	"github.com/psviderski/uncloud/internal/secret"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"net/netip"
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

func (u *User) ManagementIP() netip.Addr {
	return network.ManagementIP(u.PublicKey())
}
