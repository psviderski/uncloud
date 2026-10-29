package image

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stringid"
	"github.com/psviderski/uncloud/internal/cli/tui"
	"github.com/psviderski/uncloud/pkg/api"
)

var imageIDRegex = regexp.MustCompile(`^(sha256:)?[a-f0-9]{4,64}$`)

// imageRemovalPlan contains all image removal operations grouped by the user-supplied image name, ID, or pattern.
type imageRemovalPlan struct {
	GroupPlans []*removeImageGroupPlan
}

func (p *imageRemovalPlan) IsEmpty() bool { return len(p.GroupPlans) == 0 }

func (p *imageRemovalPlan) Format() string {
	var out strings.Builder
	machines := make(map[string]struct{})
	removeCount := 0
	for _, group := range p.GroupPlans {
		out.WriteString(group.Format())
		for _, op := range group.Operations {
			machines[op.MachineName] = struct{}{}
			removeCount++
		}
	}

	summary := fmt.Sprintf("%s remove %s across %s %s",
		tui.BoldRed.Render(fmt.Sprint(removeCount)),
		tui.Faint.Render("·"),
		tui.Bold.Render(fmt.Sprint(len(machines))),
		pluralMachines(len(machines)),
	)
	out.WriteString("\n" + tui.Faint.Render(strings.Repeat("─", lipgloss.Width(summary))) + "\n" + summary + "\n")

	return out.String()
}

func pluralMachines(count int) string {
	if count == 1 {
		return "machine"
	}
	return "machines"
}

// Execute applies the frozen plan, collecting failures without discovering new targets.
func (p *imageRemovalPlan) Execute(ctx context.Context, cli api.ImageClient) error {
	untagged := make(map[string]bool)
	var errs []error
	for _, group := range p.GroupPlans {
		if err := group.Execute(ctx, cli, untagged); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// removeImageGroupPlan groups removals for one image name, ID, or pattern supplied by the user.
type removeImageGroupPlan struct {
	// Image is the image name, ID, or pattern supplied by the user.
	Image      string
	Operations []*removeImageOperation
}

func (o *removeImageGroupPlan) Execute(ctx context.Context, cli api.ImageClient, untagged map[string]bool) error {
	var errs []error
	for _, op := range o.Operations {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := op.Execute(ctx, cli, untagged); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (o *removeImageGroupPlan) Format() string {
	var out strings.Builder
	out.WriteString(tui.BoldRed.Render("-") + " remove image " + tui.NameStyle.Render(o.Image) + "\n")
	for i, op := range o.Operations {
		branch := "  ├── "
		if i == len(o.Operations)-1 {
			branch = "  ╰── "
		}
		out.WriteString(tui.Faint.Render(branch) + op.Format() + "\n")
	}
	return out.String()
}

func (o *removeImageGroupPlan) String() string {
	return fmt.Sprintf("removeImageGroupPlan[image=%s]", o.Image)
}

// removeImageOperation removes an image name or an unnamed image by ID on a machine.
type removeImageOperation struct {
	MachineName string
	ImageID     string
	// Image is the exact image name or ID sent to Docker.
	Image string
}

func (o *removeImageOperation) Execute(ctx context.Context, cli api.ImageClient, untagged map[string]bool) error {
	if untagged[o.MachineName+"\x00"+o.Image] {
		return nil
	}
	results, err := cli.RemoveImage(ctx, o.Image, api.RemoveImageOptions{Machines: []string{o.MachineName}})
	// A removal may also untag other planned names. Track actual results across all groups, per machine.
	for _, result := range results {
		for _, removal := range result.Results {
			if removal.Untagged != "" {
				untagged[result.Metadata.MachineName+"\x00"+removal.Untagged] = true
			}
		}
	}
	if errors.Is(err, api.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove image '%s': %w", o.Image, err)
	}
	return nil
}

func (o *removeImageOperation) Format() string {
	return tui.BoldRed.Render("-") + " " + tui.FormatImage(o.Image, tui.NoStyle) +
		tui.Faint.Render(" ("+stringid.TruncateID(o.ImageID)+") on ") + o.MachineName
}

func (o *removeImageOperation) String() string {
	return fmt.Sprintf("removeImageOperation[machine_name=%s image=%s]", o.MachineName, o.Image)
}

type machineImageInventory struct {
	machineName string
	images      []image.Summary
}

// planImageRemoval builds a removal plan grouped by input argument from images on the selected machines.
// An empty machines list selects all machines.
//
// Arguments expand as follows:
//   - Name: without a tag or digest, select all stored names in that exact repository across image IDs.
//     An explicit tag selects only that tag. Familiar names are normalised.
//   - ID: select all RepoTags for the matching image, or its full ID if unnamed. A prefix must identify
//     one unique full ID across the selected machines.
//   - Pattern: match RepoTags only. A matching digest also selects that image's names in the same repository.
//   - Digest: resolve through RepoTags, RepoDigests, or the image ID, then select that image's names in
//     the same repository. If none exist but RepoDigests contains the reference, select the digest itself.
//
// Selecting all tags for a repository/image on a machine also adds its stored digest references after
// the tags, even if those digests did not match an argument. Stored names are kept as is, and overlapping
// selections produce only one operation per machine and name.
//
// Failed image listings produce warnings and exclude those machines from the plan. If no machine can
// be queried, planning fails. Every argument must match on a successfully queried machine. Unmatched
// or invalid arguments and ambiguous ID prefixes reject the whole plan before any removal.
//
// Planning does not check container usage. The backend checks each removal during execution, and failures
// do not stop other operations. Tag removal may implicitly untag a used digest. Execution skips names
// already untagged by earlier operations and tolerates names that disappeared after planning.
func planImageRemoval(ctx context.Context, cli api.ImageClient, images, machines []string) (*imageRemovalPlan, error) {
	if len(images) == 0 {
		return nil, errors.New("at least one image is required")
	}

	var clusterImages []api.MachineImages
	err := tui.RunSpinner(ctx, "Finding images to remove...", func(ctx context.Context) error {
		var err error
		clusterImages, err = cli.ListImages(ctx, api.ImageFilter{Machines: machines})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	var inventory []*machineImageInventory
	for _, machineImages := range clusterImages {
		if err := machineImages.Metadata.Err(); err != nil {
			tui.PrintWarning(fmt.Sprintf("failed to list images on machine '%s': %s",
				machineImages.Metadata.MachineName, err))
			continue
		}
		machine := &machineImageInventory{
			machineName: machineImages.Metadata.MachineName,
			images:      machineImages.Images,
		}
		inventory = append(inventory, machine)
	}
	if len(inventory) == 0 {
		return nil, errors.New("failed to list images on any machine")
	}

	return planImageRemovalFromInventory(images, inventory)
}

func planImageRemovalFromInventory(images []string, inventory []*machineImageInventory) (*imageRemovalPlan, error) {
	plan := &imageRemovalPlan{}
	seen := make(map[string]bool)
	var errs []error
	for _, imageArg := range images {
		group := &removeImageGroupPlan{Image: imageArg}
		isID := imageIDRegex.MatchString(imageArg)
		isPattern := strings.ContainsAny(imageArg, "*?[")
		var namedRef reference.Named
		if isPattern {
			if _, err := path.Match(imageArg, ""); err != nil {
				return nil, fmt.Errorf("invalid image pattern '%s': %w", imageArg, err)
			}
		} else if !isID {
			var err error
			namedRef, err = reference.ParseNormalizedNamed(imageArg)
			if err != nil {
				return nil, fmt.Errorf("invalid image '%s': %w", imageArg, err)
			}
		}
		if isID {
			matches := make(map[string]bool)
			for _, machine := range inventory {
				for _, img := range machine.images {
					if imageIDMatches(img.ID, imageArg) {
						matches[img.ID] = true
					}
				}
			}
			if len(matches) > 1 {
				var candidates []string
				for id := range matches {
					candidates = append(candidates, id)
				}
				slices.Sort(candidates)
				errs = append(errs,
					fmt.Errorf("image ID '%s' is ambiguous: %s", imageArg, strings.Join(candidates, ", ")))
				continue
			}
		}

		matched := false
		for _, machine := range inventory {
			for _, img := range machine.images {
				var imagesToRemove []string
				switch {
				case isID:
					if !imageIDMatches(img.ID, imageArg) {
						continue
					}
					matched = true
					// Expand only stored names, matching uc image ls. RepoDigests remains available for digest lookup.
					imagesToRemove = img.RepoTags
					if len(imagesToRemove) == 0 {
						imagesToRemove = []string{img.ID}
					}
				case isPattern:
					for _, ref := range img.RepoTags {
						if ok, _ := path.Match(imageArg, ref); ok {
							named, _ := reference.ParseDockerRef(ref)
							if _, digest := named.(reference.Canonical); digest {
								imagesToRemove = append(imagesToRemove,
									imageRepositoryReferences(img.RepoTags, named.Name())...)
							} else {
								imagesToRemove = append(imagesToRemove, ref)
							}
						}
					}
				case reference.IsNameOnly(namedRef):
					imagesToRemove = imageRepositoryReferences(img.RepoTags, namedRef.Name())
				default:
					if canonical, digest := namedRef.(reference.Canonical); digest {
						ref := reference.FamiliarString(namedRef)
						if canonical.Digest().String() != img.ID && !slices.Contains(img.RepoTags, ref) &&
							!slices.Contains(img.RepoDigests, ref) {
							continue
						}
						imagesToRemove = imageRepositoryReferences(img.RepoTags, namedRef.Name())
						if len(imagesToRemove) == 0 && slices.Contains(img.RepoDigests, ref) {
							// An explicitly requested digest can resolve even when no stored name is listed.
							imagesToRemove = []string{ref}
						}
					} else if slices.Contains(img.RepoTags, reference.FamiliarString(namedRef)) {
						imagesToRemove = []string{reference.FamiliarString(namedRef)}
					}
				}
				for _, ref := range imagesToRemove {
					matched = true
					key := machine.machineName + "\x00" + ref
					if !seen[key] {
						seen[key] = true
						group.Operations = append(group.Operations, &removeImageOperation{
							MachineName: machine.machineName, ImageID: img.ID, Image: ref,
						})
					}
				}
			}
		}
		if !matched {
			errs = append(errs, fmt.Errorf("image '%s' not found on any machine", imageArg))
		}
		slices.SortFunc(group.Operations, compareImageRemovalOperations)
		if len(group.Operations) > 0 {
			plan.GroupPlans = append(plan.GroupPlans, group)
		}
	}

	planDigestRemovals(plan, inventory)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return plan, nil
}

func imageRepositoryReferences(refs []string, repository string) []string {
	var matches []string
	for _, ref := range refs {
		if named, err := reference.ParseDockerRef(ref); err == nil && named.Name() == repository {
			matches = append(matches, ref)
		}
	}
	return matches
}

// planDigestRemovals includes stored digests with the group removing their last repository tag.
// RepoDigests also contains synthesised references, so only digest names stored in RepoTags get operations here.
func planDigestRemovals(plan *imageRemovalPlan, inventory []*machineImageInventory) {
	type repository struct {
		remainingTags int
		digests       []string
	}
	repositories := make(map[string]*repository)
	for _, machine := range inventory {
		for _, img := range machine.images {
			names := slices.Clone(img.RepoTags)
			slices.Sort(names)
			for _, ref := range slices.Compact(names) {
				named, err := reference.ParseDockerRef(ref)
				if err != nil {
					continue
				}
				key := machine.machineName + "\x00" + img.ID + "\x00" + named.Name()
				repo := repositories[key]
				if repo == nil {
					repo = &repository{}
					repositories[key] = repo
				}
				if _, tagged := named.(reference.Tagged); tagged {
					repo.remainingTags++
				} else if _, digest := named.(reference.Canonical); digest {
					repo.digests = append(repo.digests, ref)
				}
			}
		}
	}
	emitted := make(map[string]bool)
	var groups []*removeImageGroupPlan
	for _, group := range plan.GroupPlans {
		var operations []*removeImageOperation
		for _, op := range group.Operations {
			key := op.MachineName + "\x00" + op.Image
			if emitted[key] {
				continue
			}
			emitted[key] = true
			operations = append(operations, op)
			named, err := reference.ParseDockerRef(op.Image)
			if err != nil {
				continue
			}
			if _, tagged := named.(reference.Tagged); !tagged {
				continue
			}
			repo := repositories[op.MachineName+"\x00"+op.ImageID+"\x00"+named.Name()]
			if repo == nil {
				continue
			}
			repo.remainingTags--
			if repo.remainingTags != 0 {
				continue
			}
			for _, digest := range repo.digests {
				key := op.MachineName + "\x00" + digest
				if !emitted[key] {
					emitted[key] = true
					operations = append(operations, &removeImageOperation{
						MachineName: op.MachineName, ImageID: op.ImageID, Image: digest,
					})
				}
			}
		}
		group.Operations = operations
		slices.SortFunc(group.Operations, compareImageRemovalOperations)
		if len(group.Operations) > 0 {
			groups = append(groups, group)
		}
	}
	plan.GroupPlans = groups
}

func compareImageRemovalOperations(a, b *removeImageOperation) int {
	aDigest, bDigest := strings.Contains(a.Image, "@"), strings.Contains(b.Image, "@")
	if aDigest != bDigest {
		if aDigest {
			return 1
		}
		return -1
	}
	if c := strings.Compare(a.Image, b.Image); c != 0 {
		return c
	}
	return strings.Compare(a.MachineName, b.MachineName)
}

func imageIDMatches(id, prefix string) bool {
	return strings.HasPrefix(strings.TrimPrefix(id, "sha256:"), strings.TrimPrefix(prefix, "sha256:"))
}
