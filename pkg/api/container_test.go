package api

import (
	"net/netip"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainer_Healthy(t *testing.T) {
	t.Parallel()

	t.Run("exited", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Running:  false,
					Dead:     false,
					ExitCode: 0,
				},
			},
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("running with no health check", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Running: true,
				},
			},
		}}
		assert.True(t, c.Healthy())
	})

	t.Run("running and healthy", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Running: true,
					Health: &types.Health{
						Status: types.Healthy,
					},
				},
			},
		}}
		assert.True(t, c.Healthy())
	})

	t.Run("running but unhealthy", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Running: true,
					Health: &types.Health{
						Status: types.Unhealthy,
					},
				},
			},
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("running with health starting", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Running: true,
					Health: &types.Health{
						Status: "starting",
					},
				},
			},
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("dead", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Dead:    true,
					Running: false,
				},
			},
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("restarting", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Restarting: true,
					Running:    true,
					ExitCode:   1,
				},
			},
		}}
		assert.False(t, c.Healthy())
	})

	t.Run("paused", func(t *testing.T) {
		t.Parallel()
		c := &Container{ContainerJSON: types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				State: &types.ContainerState{
					Paused:  true,
					Running: true,
				},
			},
		}}
		assert.False(t, c.Healthy())
	})
}

func TestContainer_ConflictingServicePorts(t *testing.T) {
	tests := []struct {
		name           string
		containerPorts string
		checkPorts     []PortSpec
		want           []PortSpec
		wantErr        bool
	}{
		{
			name:           "no conflicts when container has no ports",
			containerPorts: "",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolTCP},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "host mode ports with same published port and protocol conflict",
			containerPorts: "8080:80/tcp@host",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolTCP},
			},
			want: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolTCP},
			},
			wantErr: false,
		},
		{
			name:           "host mode ports with same port but different protocols don't conflict",
			containerPorts: "8080:80/tcp@host",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolUDP},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "multiple protocols on same port don't conflict",
			containerPorts: "8080:80/tcp@host,8080:80/udp@host",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolUDP},
			},
			want: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolUDP},
			},
			wantErr: false,
		},
		{
			name:           "host mode ports with different published ports don't conflict",
			containerPorts: "8080:80/tcp@host",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8081, ContainerPort: 80, Protocol: ProtocolTCP},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "host mode ports with same published port but different host IPs don't conflict",
			containerPorts: "127.0.0.1:8080:80/tcp@host",
			checkPorts: []PortSpec{
				{
					Mode:          PortModeHost,
					HostIP:        netip.MustParseAddr("127.0.0.2"),
					PublishedPort: 8080,
					ContainerPort: 80,
					Protocol:      ProtocolTCP,
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "host mode ports with same published port, protocol, and host IP conflict",
			containerPorts: "127.0.0.1:8080:80/tcp@host",
			checkPorts: []PortSpec{
				{
					Mode:          PortModeHost,
					HostIP:        netip.MustParseAddr("127.0.0.1"),
					PublishedPort: 8080,
					ContainerPort: 80,
					Protocol:      ProtocolTCP,
				},
			},
			want: []PortSpec{
				{
					Mode:          PortModeHost,
					HostIP:        netip.MustParseAddr("127.0.0.1"),
					PublishedPort: 8080,
					ContainerPort: 80,
					Protocol:      ProtocolTCP,
				},
			},
			wantErr: false,
		},
		{
			name:           "host mode port with no host IP conflicts with specific host IP on same port and protocol",
			containerPorts: "8080:80/tcp@host",
			checkPorts: []PortSpec{
				{
					Mode:          PortModeHost,
					HostIP:        netip.MustParseAddr("127.0.0.1"),
					PublishedPort: 8080,
					ContainerPort: 80,
					Protocol:      ProtocolTCP,
				},
			},
			want: []PortSpec{
				{
					Mode:          PortModeHost,
					HostIP:        netip.MustParseAddr("127.0.0.1"),
					PublishedPort: 8080,
					ContainerPort: 80,
					Protocol:      ProtocolTCP,
				},
			},
			wantErr: false,
		},
		{
			name:           "host mode port with no host IP doesn't conflict with different protocol",
			containerPorts: "8080:80/tcp@host",
			checkPorts: []PortSpec{
				{
					Mode:          PortModeHost,
					HostIP:        netip.MustParseAddr("127.0.0.1"),
					PublishedPort: 8080,
					ContainerPort: 80,
					Protocol:      ProtocolUDP,
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "ingress mode ports don't conflict with host mode ports",
			containerPorts: "8080:80/tcp",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolTCP},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "container with invalid port spec returns error",
			containerPorts: "invalid:port:spec",
			checkPorts: []PortSpec{
				{Mode: PortModeHost, PublishedPort: 8080, ContainerPort: 80, Protocol: ProtocolTCP},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := &ServiceContainer{Container: Container{ContainerJSON: types.ContainerJSON{
				Config: &container.Config{
					Labels: map[string]string{
						LabelServicePorts: tt.containerPorts,
					},
				},
			}}}

			got, err := ctr.ConflictingServicePorts(tt.checkPorts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
