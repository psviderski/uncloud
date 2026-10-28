package api

import (
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/image"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/psviderski/uncloud/api/pb"
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

// ImageReferences returns the image tag and named digest references, sorted and without duplicates.
// References use familiar names such as "alpine:latest" and "alpine@sha256:...".
// Bare digests and invalid references are excluded. It returns nil if there are no named references.
func ImageReferences(img image.Summary) []string {
	var refs []string
	for _, names := range [][]string{img.RepoTags, img.RepoDigests} {
		for _, name := range names {
			ref, err := reference.ParseAnyReference(name)
			if err != nil {
				continue
			}
			// RepoDigests can contain bare digests. Keep only references with a repository name.
			if named, ok := ref.(reference.Named); ok {
				refs = append(refs, reference.FamiliarString(named))
			}
		}
	}

	slices.Sort(refs)
	return slices.Compact(refs)
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
