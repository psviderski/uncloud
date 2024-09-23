package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"github.com/siderolabs/discovery-api/api/v1alpha1/client/pb"
	discovery "github.com/siderolabs/discovery-client/pkg/client"
	"go.uber.org/zap"
	"net/netip"
	"time"
	"uncloud/internal/machine/network"
)

const (
	defaultDiscoveryTTL = 30 * time.Minute
)

func TestDiscovery() error {
	addrs, err := network.ListRoutableIPs()
	if err != nil {
		return err
	}

	clusterID := "test-cluster"
	secretBytes, _ := hex.DecodeString("08841038cc5a81e26ee794ebe8821d681bfea7a118ba5e25416e38884bf55c47")

	var cipherBlock cipher.Block
	cipherBlock, err = aes.NewCipher(secretBytes)
	if err != nil {
		return fmt.Errorf("create AES cipher: %w", err)
	}

	client, err := discovery.NewClient(
		discovery.Options{
			Cipher:        cipherBlock,
			Endpoint:      "discovery.talos.dev:443",
			ClusterID:     clusterID,
			AffiliateID:   "machine1",
			TTL:           defaultDiscoveryTTL,
			ClientVersion: "v1.7.0", // Talos version tag.
		},
	)
	if err != nil {
		return fmt.Errorf("init discovery client: %w", err)
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	notifyCh := make(chan struct{}, 1)

	go func() {
		logger, _ := zap.NewDevelopment()
		err = client.Run(clientCtx, logger, notifyCh)
		if err != nil {
			fmt.Println("Discovery client error:", err)
		}
	}()

	// Wait for the client to be ready.
	<-notifyCh
	fmt.Println("Discovery client ready")
	var publicIP netip.Addr
	if err = publicIP.UnmarshalBinary(client.GetPublicIP()); err != nil {
		fmt.Printf("unmarshal public IP: %s", err)
	}

	fmt.Println(publicIP)

	wireguardAddress, err := netip.MustParseAddr("10.210.1.1").MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal wireguard address: %w", err)
	}
	endpoints := make([]*pb.Endpoint, len(addrs))
	for i, addr := range addrs {
		ip, mErr := addr.MarshalBinary()
		if mErr != nil {
			return fmt.Errorf("marshal address: %w", mErr)
		}
		endpoints[i] = &pb.Endpoint{
			Ip:   ip,
			Port: network.WireGuardPort,
		}
	}
	if err = client.SetLocalData(
		&discovery.Affiliate{
			Affiliate: &pb.Affiliate{
				NodeId:          "machine1",
				Addresses:       [][]byte{},
				Hostname:        "",
				Nodename:        "machine1-name",
				MachineType:     "",
				OperatingSystem: "",
				Kubespan: &pb.KubeSpan{
					PublicKey: "public-key",
					Address:   wireguardAddress,
					AdditionalAddresses: []*pb.IPPrefix{
						{Ip: wireguardAddress, Bits: 24},
					},
				},
			},
			Endpoints: endpoints,
		}, []discovery.Endpoint{},
	); err != nil {
		return fmt.Errorf("set local affiliate data: %w", err)
	}

	affiliates := client.GetAffiliates()
	for _, affiliate := range affiliates {
		fmt.Println("Affiliate:", affiliate)
	}

	time.Sleep(2 * time.Second)
	clientCancel()
	fmt.Println("Discovery client done")
	return nil
}
