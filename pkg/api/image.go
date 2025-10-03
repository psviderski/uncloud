package api

import (
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

type MachineImage struct {
	Metadata *pb.Metadata
	Image    types.ImageInspect
}

// MachineImages represents images present on a particular machine.
type MachineImages struct {
	Metadata *pb.Metadata
	// DockerImages is a list of images present in the Docker internal image store.
	// It may be empty if Docker uses the containerd image store directly (containerd-snapshotter feature).
	DockerImages []image.Summary
	// ContainerdImages is a list of images present in the containerd image store.
	ContainerdImages []image.Summary
}

// ImageFilter defines criteria to filter images in ListImages.
type ImageFilter struct {
	// Machines filters images to those present on the specified machines (names or IDs).
	// If empty, it matches images on all machines.
	Machines []string
}

// MachineRemoteImage represents an image in a remote registry fetched by a particular machine.
type MachineRemoteImage struct {
	Metadata *pb.Metadata
	Image    RemoteImage
}

// RemoteImage represents an image in a remote registry. The canonical reference includes the image digest.
// Either IndexManifest or ImageManifest must be set depending on whether the reference points to an index
// (multi-platform image) or a single-platform image.
type RemoteImage struct {
	Reference     reference.Canonical
	IndexManifest *v1.Index
	ImageManifest *v1.Manifest
}
