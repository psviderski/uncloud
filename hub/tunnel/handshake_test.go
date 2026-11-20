package tunnel

import (
	"crypto/ecdh"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeHMAC(t *testing.T) {
	t.Parallel()

	machine, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	challenge, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	machinePub := machine.PublicKey().Bytes()
	challengePub := challenge.PublicKey().Bytes()

	// The machine and Hub derive the same shared secret from their own private key and the other public key.
	machineShared, err := machine.ECDH(challenge.PublicKey())
	require.NoError(t, err)
	hubShared, err := challenge.ECDH(machine.PublicKey())
	require.NoError(t, err)

	hmac, err := computeHMAC(machineShared, "cluster", "machine", machinePub, challengePub)
	require.NoError(t, err)
	hubHMAC, err := computeHMAC(hubShared, "cluster", "machine", machinePub, challengePub)
	require.NoError(t, err)
	assert.Equal(t, hmac, hubHMAC)

	other, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	otherShared, err := other.ECDH(challenge.PublicKey())
	require.NoError(t, err)

	// Changing any input changes the MAC.
	variants := map[string]func() ([]byte, error){
		"shared secret": func() ([]byte, error) {
			return computeHMAC(otherShared, "cluster", "machine", machinePub, challengePub)
		},
		"cluster ID": func() ([]byte, error) {
			return computeHMAC(machineShared, "other", "machine", machinePub, challengePub)
		},
		"machine ID": func() ([]byte, error) {
			return computeHMAC(machineShared, "cluster", "other", machinePub, challengePub)
		},
		"field boundaries": func() ([]byte, error) {
			return computeHMAC(machineShared, "clusterm", "achine", machinePub, challengePub)
		},
		"machine key": func() ([]byte, error) {
			return computeHMAC(machineShared, "cluster", "machine",
				other.PublicKey().Bytes(), challengePub)
		},
	}
	for name, compute := range variants {
		got, err := compute()
		require.NoError(t, err)
		assert.NotEqual(t, hmac, got, name)
	}
}

func TestLowOrderKeyRejected(t *testing.T) {
	t.Parallel()

	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	// The all-zero point has a low order, so the shared secret would be zero for any private key.
	lowOrder, err := ecdh.X25519().NewPublicKey(make([]byte, 32))
	require.NoError(t, err)

	_, err = priv.ECDH(lowOrder)
	assert.Error(t, err)
}
