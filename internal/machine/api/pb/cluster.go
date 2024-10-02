package pb

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KeyLen is the expected key length for a WireGuard public or private key.
const KeyLen = 32

func (c *NetworkConfig) Validate() error {
	if c.Subnet != nil {
		_, err := c.Subnet.ToPrefix()
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid subnet: %v", err)
		}
	}
	if c.ManagementIp != nil {
		_, err := c.ManagementIp.ToAddr()
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid management IP: %v", err)
		}
	}
	for _, ep := range c.Endpoints {
		if ep == nil {
			return status.Error(codes.InvalidArgument, "endpoint not set")
		}
		_, err := ep.ToAddrPort()
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid endpoint: %v", err)
		}
	}
	if c.PublicKey == nil {
		return status.Error(codes.InvalidArgument, "public key not set")
	}
	if len(c.PublicKey) != KeyLen {
		return status.Errorf(codes.InvalidArgument, "invalid public key length: %d", len(c.PublicKey))
	}

	return nil
}
