package api

import (
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceSpec_Validate_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty allows generated name"},
		{name: "single character", input: "a"},
		{name: "single digit", input: "1"},
		{name: "two characters", input: "a1"},
		{name: "hyphens and digits", input: "1-web-2"},
		{name: "consecutive hyphens", input: "web--1"},
		{name: "maximum length", input: strings.Repeat("a", 63)},
		{name: "maximum length with hyphens", input: "a" + strings.Repeat("-", 61) + "1"},
		{name: "round-robin mode is a valid service name", input: "rr"},
		{name: "nearest mode is a valid service name", input: "nearest"},
		{name: "machine namespace prefix", input: "m-service"},
		{name: "short hexadecimal name", input: strings.Repeat("a", 31)},
		{name: "long hexadecimal name", input: strings.Repeat("a", 33)},
		{name: "non-hexadecimal 32 characters", input: strings.Repeat("g", 32)},
		{name: "too long", input: strings.Repeat("a", 64), wantErr: "must be 1-63 characters"},
		{name: "uppercase", input: "WEB1", wantErr: "lowercase letters"},
		{name: "leading hyphen", input: "-web", wantErr: "starting and ending"},
		{name: "trailing hyphen", input: "web-", wantErr: "starting and ending"},
		{name: "hyphen only", input: "-", wantErr: "starting and ending"},
		{name: "underscore", input: "web_1", wantErr: "hyphens only"},
		{name: "dot", input: "web.example", wantErr: "hyphens only"},
		{name: "slash", input: "web/api", wantErr: "hyphens only"},
		{name: "space", input: "web 1", wantErr: "hyphens only"},
		{name: "leading whitespace", input: " web", wantErr: "hyphens only"},
		{name: "trailing whitespace", input: "web ", wantErr: "hyphens only"},
		{name: "tab", input: "web\t1", wantErr: "hyphens only"},
		{name: "newline", input: "web\n", wantErr: "hyphens only"},
		{name: "non-ASCII", input: "wéb", wantErr: "lowercase letters"},
		{name: "machine DNS namespace", input: "m", wantErr: "reserved for the machine DNS namespace"},
		{name: "service ID", input: "c337f00600de51ef4375c9a9a267dba5", wantErr: "service ID format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := ServiceSpec{Name: tt.input, Container: ContainerSpec{Image: "nginx:latest"}}
			err := spec.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestServiceSpec_Validate_CaddyAndPorts(t *testing.T) {
	tests := []struct {
		name    string
		spec    ServiceSpec
		wantErr string
	}{
		{
			name: "valid with neither Caddy nor Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
			},
			wantErr: "",
		},
		{
			name: "valid with Caddy only",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
			},
			wantErr: "",
		},
		{
			name: "valid with Ports only",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid with empty Caddy config and Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
					},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid with Caddy and Ports (default mode is ingress)",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
						// Mode is empty, defaults to ingress
					},
				},
			},
			wantErr: "ingress ports and Caddy configuration cannot be specified simultaneously",
		},
		{
			name: "invalid with both Caddy and ingress Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
						Mode:          PortModeIngress,
					},
				},
			},
			wantErr: "ingress ports and Caddy configuration cannot be specified simultaneously",
		},
		{
			name: "valid with Caddy and host mode Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 3306,
						PublishedPort: 3306,
						Protocol:      ProtocolTCP,
						Mode:          PortModeHost,
					},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid with Caddy and mixed mode Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 3306,
						PublishedPort: 3306,
						Protocol:      ProtocolTCP,
						Mode:          PortModeHost,
					},
					{
						ContainerPort: 80,
						Protocol:      ProtocolHTTP,
						Mode:          PortModeIngress,
					},
				},
			},
			wantErr: "ingress ports and Caddy configuration cannot be specified simultaneously",
		},
		{
			name: "valid with Caddy and multiple host mode Ports",
			spec: ServiceSpec{
				Name: "test",
				Container: ContainerSpec{
					Image: "nginx:latest",
				},
				Caddy: &CaddySpec{
					Config: "example.com {\n  reverse_proxy :8080\n}",
				},
				Ports: []PortSpec{
					{
						ContainerPort: 3306,
						PublishedPort: 3306,
						Protocol:      ProtocolTCP,
						Mode:          PortModeHost,
					},
					{
						ContainerPort: 5432,
						PublishedPort: 5432,
						Protocol:      ProtocolTCP,
						Mode:          PortModeHost,
					},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestContainerSpec_Clone(t *testing.T) {
	mode := os.FileMode(0o644)
	original := ContainerSpec{
		CapAdd:     []string{"NET_ADMIN"},
		CapDrop:    []string{"ALL"},
		PidMode:    "host",
		Command:    []string{"sh", "-c", "echo hello"},
		Entrypoint: []string{"/bin/bash"},
		Env: EnvVars{
			"FOO": "bar",
			"BAZ": "qux",
		},
		Image: "nginx:latest",
		Init:  new(true),
		LogDriver: &LogDriver{
			Name: "json-file",
			Options: map[string]string{
				"max-size": "10m",
			},
		},
		Privileged: true,
		PullPolicy: PullPolicyAlways,
		Resources: ContainerResources{
			CPU:               1234,
			Memory:            2345,
			MemoryReservation: 3456,
			Devices: []DeviceMapping{
				{HostPath: "/dev/sda", ContainerPath: "/dev/xvda", CgroupPermissions: "rwm"},
			},
			DeviceReservations: []container.DeviceRequest{
				{Count: 1, Capabilities: [][]string{{"gpu"}}, Driver: "nvidia"},
			},
		},
		Sysctls: map[string]string{
			"net.ipv4.ip_forward": "1",
		},
		User:    "1000:1000",
		Volumes: []string{"/data", "/config"},
		VolumeMounts: []VolumeMount{
			{VolumeName: "data", ContainerPath: "/data"},
		},
		ConfigMounts: []ConfigMount{
			{ConfigName: "app-config", ContainerPath: "/etc/config", Mode: &mode},
		},
	}

	cloned := original.Clone()

	// Check ContainerSpec equality
	assert.True(t, original.Equals(cloned))

	// Verify deep copy by modifying the original
	stringModified := "modified"
	original.CapAdd[0] = stringModified
	original.CapDrop[0] = stringModified
	original.Command[0] = stringModified
	original.Entrypoint[0] = stringModified
	original.Env["FOO"] = stringModified
	original.LogDriver.Options["max-size"] = stringModified
	original.Volumes[0] = stringModified
	original.VolumeMounts[0].ContainerPath = stringModified
	original.ConfigMounts[0].ContainerPath = stringModified
	*original.ConfigMounts[0].Mode = 0o755 // Modify the Mode pointer value
	original.Sysctls["net.ipv4.ip_forward"] = stringModified
	original.Resources.Devices[0].HostPath = stringModified
	original.Resources.DeviceReservations[0].Count = 2
	original.Resources.DeviceReservations[0].Driver = stringModified

	assert.False(t, original.Equals(cloned))
	// Assert cloned values are unchanged
	assert.Equal(t, "NET_ADMIN", cloned.CapAdd[0])
	assert.Equal(t, "ALL", cloned.CapDrop[0])
	assert.Equal(t, "sh", cloned.Command[0])
	assert.Equal(t, "/bin/bash", cloned.Entrypoint[0])
	assert.Equal(t, "bar", cloned.Env["FOO"])
	assert.Equal(t, "qux", cloned.Env["BAZ"])
	assert.Equal(t, "nginx:latest", cloned.Image)
	assert.NotNil(t, cloned.Init)
	assert.Equal(t, true, *cloned.Init)
	assert.NotNil(t, cloned.LogDriver)
	assert.Equal(t, "json-file", cloned.LogDriver.Name)
	assert.Equal(t, "10m", cloned.LogDriver.Options["max-size"])
	assert.Equal(t, true, cloned.Privileged)
	assert.Equal(t, PullPolicyAlways, cloned.PullPolicy)
	assert.Equal(t, int64(1234), cloned.Resources.CPU)
	assert.Equal(t, int64(2345), cloned.Resources.Memory)
	assert.Equal(t, int64(3456), cloned.Resources.MemoryReservation)
	assert.Equal(t, "/dev/sda", cloned.Resources.Devices[0].HostPath)
	assert.Equal(t, 1, cloned.Resources.DeviceReservations[0].Count)
	assert.Equal(t, "nvidia", cloned.Resources.DeviceReservations[0].Driver)
	assert.Equal(t, "1000:1000", cloned.User)
	assert.Equal(t, "/data", cloned.Volumes[0])
	assert.Equal(t, "/data", cloned.VolumeMounts[0].ContainerPath)
	assert.Equal(t, "/etc/config", cloned.ConfigMounts[0].ContainerPath)
	assert.NotNil(t, cloned.ConfigMounts[0].Mode)
	assert.Equal(t, os.FileMode(0o644), *cloned.ConfigMounts[0].Mode, "Mode should be deep copied")
	assert.Equal(t, "1", cloned.Sysctls["net.ipv4.ip_forward"])
}
