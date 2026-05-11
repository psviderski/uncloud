package caddyconfig

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
)

// TestContainerFingerprint_EqualCoversAllFields is a guard: when a field is added to containerFingerprint,
// Equal must also compare it. A mutation of any single field should flip equality to false. If this test fails
// after adding a field, update Equal to include it.
func TestContainerFingerprint_EqualCoversAllFields(t *testing.T) {
	t.Parallel()

	base := containerFingerprint{
		ID: "container-1",
		IP: netip.MustParseAddr("10.210.0.2"),
		Ports: []api.PortSpec{{
			Hostname:      "app.example.com",
			ContainerPort: 8080,
			Protocol:      api.ProtocolHTTP,
			Mode:          api.PortModeIngress,
		}},
		CaddyConfig: "caddy-config",
	}

	assert.True(t, base.Equal(base), "base fingerprint must be equal to itself")

	rt := reflect.TypeOf(base)
	for field := range rt.Fields() {
		t.Run(field.Name, func(t *testing.T) {
			t.Parallel()

			mutated := base
			mutated.Ports = append([]api.PortSpec(nil), base.Ports...)
			v := reflect.ValueOf(&mutated).Elem().FieldByName(field.Name)

			switch field.Name {
			case "ID", "CaddyConfig":
				v.SetString(v.String() + "-changed")
			case "IP":
				v.Set(reflect.ValueOf(netip.MustParseAddr("10.210.0.99")))
			case "Ports":
				mutated.Ports = []api.PortSpec{{
					Hostname:      "different.example.com",
					ContainerPort: 9090,
					Protocol:      api.ProtocolHTTP,
					Mode:          api.PortModeIngress,
				}}
			default:
				t.Fatalf("containerFingerprint has a new field %q without a mutation case in this test. "+
					"Add a case here and make sure Equal() compares it.", field.Name)
			}

			assert.False(t, base.Equal(mutated),
				"changing %q must flip Equal to false. Update containerFingerprint.Equal to compare it.",
				field.Name)
		})
	}
}
