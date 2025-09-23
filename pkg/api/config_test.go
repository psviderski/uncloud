package api

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uint64Ptr is a convenience function to create a pointer to a uint64 value
func uint64Ptr(v uint64) *uint64 {
	return &v
}

func TestConfigMount_GetNumericUid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		uid      string
		expected *uint64
		wantErr  string
	}{
		{
			name:     "empty uid returns nil",
			uid:      "",
			expected: nil,
		},
		{
			name:     "valid numeric uid",
			uid:      "1000",
			expected: uint64Ptr(1000),
		},
		{
			name:     "zero uid",
			uid:      "0",
			expected: uint64Ptr(0),
		},
		{
			name:    "invalid non-numeric uid",
			uid:     "root",
			wantErr: "invalid Uid 'root'",
		},
		{
			name:    "negative uid",
			uid:     "-1",
			wantErr: "invalid Uid",
		},
		{
			name:     "very large uid",
			uid:      "18446744073709551615", // max uint64
			expected: uint64Ptr(18446744073709551615),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mount := &ConfigMount{Uid: tt.uid}
			uid, err := mount.GetNumericUid()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, uid)
				return
			}

			require.NoError(t, err)
			if tt.expected == nil {
				assert.Nil(t, uid)
			} else {
				require.NotNil(t, uid)
				assert.Equal(t, *tt.expected, *uid)
			}
		})
	}
}

func TestConfigMount_GetNumericGid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		gid      string
		expected *uint64
		wantErr  string
	}{
		{
			name:     "empty gid returns nil",
			gid:      "",
			expected: nil,
		},
		{
			name:     "valid numeric gid",
			gid:      "1000",
			expected: uint64Ptr(1000),
		},
		{
			name:     "zero gid",
			gid:      "0",
			expected: uint64Ptr(0),
		},
		{
			name:    "invalid non-numeric gid",
			gid:     "wheel",
			wantErr: "invalid Gid 'wheel'",
		},
		{
			name:    "negative gid",
			gid:     "-1",
			wantErr: "invalid Gid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mount := &ConfigMount{Gid: tt.gid}
			gid, err := mount.GetNumericGid()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, gid)
				return
			}

			require.NoError(t, err)
			if tt.expected == nil {
				assert.Nil(t, gid)
			} else {
				require.NotNil(t, gid)
				assert.Equal(t, *tt.expected, *gid)
			}
		})
	}
}

func TestValidateConfigsAndMounts(t *testing.T) {
	t.Parallel()

	mode := os.FileMode(0o644)

	tests := []struct {
		name    string
		configs []ConfigSpec
		mounts  []ConfigMount
		wantErr string
	}{
		{
			name:    "empty configs and mounts",
			configs: []ConfigSpec{},
			mounts:  []ConfigMount{},
		},
		{
			name: "valid configs without mounts",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
				{Name: "config2", Content: []byte("content2")},
			},
			mounts: []ConfigMount{},
		},
		{
			name: "valid configs with valid mounts",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
				{Name: "config2", Content: []byte("content2")},
			},
			mounts: []ConfigMount{
				{ConfigName: "config1", ContainerPath: "/etc/config1"},
				{ConfigName: "config2", ContainerPath: "/etc/config2", Uid: "1000", Gid: "1000"},
			},
		},
		{
			name: "config with empty name",
			configs: []ConfigSpec{
				{Name: "", Content: []byte("content")},
			},
			mounts:  []ConfigMount{},
			wantErr: "config name is required",
		},
		{
			name: "duplicate config names",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
				{Name: "config1", Content: []byte("content2")},
			},
			mounts:  []ConfigMount{},
			wantErr: "duplicate config name: 'config1'",
		},
		{
			name: "mount with empty config name",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "", ContainerPath: "/etc/config"},
			},
			wantErr: "config mount source is required",
		},
		{
			name: "mount referencing non-existent config",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "nonexistent", ContainerPath: "/etc/config"},
			},
			wantErr: "config mount source 'nonexistent' does not refer to any defined config",
		},
		{
			name: "mount with invalid uid",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "config1", ContainerPath: "/etc/config", Uid: "invalid"},
			},
			wantErr: "invalid Uid 'invalid'",
		},
		{
			name: "mount with invalid gid",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "config1", ContainerPath: "/etc/config", Gid: "invalid"},
			},
			wantErr: "invalid Gid 'invalid'",
		},
		{
			name: "mount with relative container path",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "config1", ContainerPath: "relative/path"},
			},
			wantErr: "container path must be absolute",
		},
		{
			name: "mount with empty container path",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "config1", ContainerPath: ""},
			},
			// Empty path is allowed
		},
		{
			name: "mount with absolute container path",
			configs: []ConfigSpec{
				{Name: "config1", Content: []byte("content1")},
			},
			mounts: []ConfigMount{
				{ConfigName: "config1", ContainerPath: "/absolute/path"},
			},
		},
		{
			name: "complex valid scenario",
			configs: []ConfigSpec{
				{Name: "nginx-conf", Content: []byte("server { listen 80; }")},
				{Name: "app-config", Content: []byte("debug=true")},
				{Name: "cert", Content: []byte("-----BEGIN CERTIFICATE-----")},
			},
			mounts: []ConfigMount{
				{ConfigName: "nginx-conf", ContainerPath: "/etc/nginx/nginx.conf", Uid: "0", Gid: "0", Mode: &mode},
				{ConfigName: "app-config", ContainerPath: "/app/config.env"},
				{ConfigName: "cert", ContainerPath: "/etc/ssl/cert.pem", Uid: "1000", Gid: "1000"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateConfigsAndMounts(tt.configs, tt.mounts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}
