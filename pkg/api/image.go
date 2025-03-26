package api

import (
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
)

type MachineImage struct {
	Metadata *pb.Metadata
	Image    types.ImageInspect
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
