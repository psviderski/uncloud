package jsonconv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCamelCase(t *testing.T) {
	assert.Equal(t, camelCase("bla"), "Bla")
	assert.Equal(t, camelCase("public_key"), "PublicKey")
	assert.Equal(t, camelCase("MachineID"), "MachineID")
	assert.Equal(t, camelCase("p"), "P")
	assert.Equal(t, camelCase("network_id"), "NetworkID")
	assert.Equal(t, camelCase("uncloud.managed"), "uncloud.managed")
}
