package api

import (
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/jsonmessage"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

type MachineImage struct {
	Metadata *pb.Metadata
	Image    image.InspectResponse
}

// MachineImages represents images present on a particular machine.
type MachineImages struct {
	Metadata *pb.Metadata
	// Images is a list of images present on the machine.
	Images []image.Summary
	// ContainerdStore indicates whether Docker on the machine uses the containerd image store
	// (containerd-snapshotter feature).
	ContainerdStore bool
}

// ImageFilter defines criteria to filter images in ListImages.
type ImageFilter struct {
	// Machines filters images to those present on the specified machines (names or IDs).
	// If empty, it matches images on all machines.
	Machines []string
	// Name filters images by name (with or without tag). Accepts a wildcard pattern.
	// If empty, it matches all image names.
	Name string
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

// MachineRemoveImageResponse represents the response from removing an image on a particular machine.
type MachineRemoveImageResponse struct {
	Metadata *pb.Metadata
	Response []image.DeleteResponse
}

// MachinePullImageMessage represents a progress message from pulling an image on a particular machine.
type MachinePullImageMessage struct {
	Metadata *pb.Metadata
	Message  jsonmessage.JSONMessage
	Err      error
}

// MachinePruneImagesResponse represents the response from pruning images on a particular machine.
type MachinePruneImagesResponse struct {
	Metadata *pb.Metadata
	Report   image.PruneReport
}
