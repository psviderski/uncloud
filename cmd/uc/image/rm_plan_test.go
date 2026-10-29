package image

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanImageRemovalFromInventory_Selectors(t *testing.T) {
	idA, idB := testImageID("a"), testImageID("b")
	digestRef := "myapp@" + testImageID("c")
	tests := []struct {
		name      string
		selector  string
		inventory []*machineImageInventory
		want      []removeImageOperation
	}{
		{
			name: "repository selects all stored names across images and machines", selector: "myapp",
			inventory: []*machineImageInventory{
				testMachineInventory("dev1", []image.Summary{
					testImage(idA, "myapp:old", "other:keep"),
					testImage(idB, "myapp:latest"),
					testImage(testImageID("c"), digestRef),
					testImage(testImageID("d"), "myapp-extra:latest", "team/myapp:latest", "myapp/worker:latest"),
				}),
				testMachineInventory("machine2", []image.Summary{testImage(idB, "myapp:test")}),
			},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idB, Image: "myapp:latest"},
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "machine2", ImageID: idB, Image: "myapp:test"},
				{MachineName: "dev1", ImageID: testImageID("c"), Image: digestRef},
			},
		},
		{
			name: "repository without latest includes its tags and stored digest", selector: "myapp",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", digestRef, "other:keep"),
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "dev1", ImageID: idA, Image: digestRef},
			},
		},
		{
			name: "explicit latest selects only that tag", selector: "myapp:latest",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", "myapp:latest", digestRef),
			})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:latest"}},
		},
		{
			name: "qualified repository selects familiar names", selector: "docker.io/library/myapp",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", "myapp:latest", "other:keep"),
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:latest"},
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
			},
		},
		{
			name: "registry port does not count as an explicit tag", selector: "registry.example.com:5000/team/myapp",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "registry.example.com:5000/team/myapp:old", "registry.example.com:5000/team/myapp:new",
					"registry.example.com:5001/team/myapp:keep", "registry.example.com:5000/team/myapp/worker:keep"),
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "registry.example.com:5000/team/myapp:new"},
				{MachineName: "dev1", ImageID: idA, Image: "registry.example.com:5000/team/myapp:old"},
			},
		},
		{
			name: "explicit tag with registry port stays exact", selector: "registry.example.com:5000/team/myapp:old",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "registry.example.com:5000/team/myapp:old", "registry.example.com:5000/team/myapp:new"),
			})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "registry.example.com:5000/team/myapp:old"}},
		},
		{
			name: "tag uses each machines local image and allows missing copies", selector: "myapp:old",
			inventory: []*machineImageInventory{
				testMachineInventory("machine2", []image.Summary{testImage(idB, "myapp:old")}),
				testMachineInventory("empty", nil),
				testMachineInventory("dev1", []image.Summary{testImage(idA, "myapp:old")}),
			},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "machine2", ImageID: idB, Image: "myapp:old"},
			},
		},
		{
			name: "qualified names use familiar references", selector: "docker.io/library/myapp:old",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old"),
			})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:old"}},
		},
		{
			name: "full ID expands all repositories and deduplicates references per machine", selector: idA,
			inventory: []*machineImageInventory{
				testMachineInventory("dev1", []image.Summary{
					{
						ID:       idA,
						RepoTags: []string{"other:keep", "myapp:old", "myapp:old", digestRef, "other@" + testImageID("c")},
						RepoDigests: []string{
							digestRef, "other@" + testImageID("c"), "other@" + testImageID("c"), testImageID("c"),
						},
					},
					testImage(idB, "unrelated:keep"),
				}),
				testMachineInventory("machine2", []image.Summary{{
					ID: idA, RepoTags: []string{"myapp:old", digestRef}, RepoDigests: []string{digestRef},
				}}),
			},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "machine2", ImageID: idA, Image: "myapp:old"},
				{MachineName: "dev1", ImageID: idA, Image: "other:keep"},
				{MachineName: "dev1", ImageID: idA, Image: digestRef},
				{MachineName: "machine2", ImageID: idA, Image: digestRef},
				{MachineName: "dev1", ImageID: idA, Image: "other@" + testImageID("c")},
			},
		},
		{
			name: "same full ID on multiple machines is unambiguous", selector: "aaaaaaaa",
			inventory: []*machineImageInventory{
				testMachineInventory("dev1", []image.Summary{testImage(idA, "myapp:old")}),
				testMachineInventory("dev1", []image.Summary{testImage(idB, "myapp:latest")}),
				testMachineInventory("machine2", []image.Summary{testImage(idA, "other:test")}),
			},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "machine2", ImageID: idA, Image: "other:test"},
			},
		},
		{
			name: "short ID with algorithm prefix", selector: "sha256:aaaaaaaa",
			inventory: []*machineImageInventory{testMachineInventory("dev1",
				[]image.Summary{testImage(idA, "myapp:old")})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:old"}},
		},
		{
			name: "unnamed image is removed by its full ID", selector: idA,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{ID: idA}})},
			want:      []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: idA}},
		},
		{
			name: "ID expands only stored names", selector: idA,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoTags: []string{"myapp:latest"},
				RepoDigests: []string{"myapp@" + idA, "other@" + idA},
			}})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:latest"}},
		},
		{
			name: "ID removes an untagged image by ID even if RepoDigests is populated", selector: idA,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoDigests: []string{digestRef},
			}})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: idA}},
		},
		{
			name: "synthesized digest selects repository tags without a digest row", selector: digestRef,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoTags: []string{"myapp:old", "other:keep", "myapp:latest"},
				RepoDigests: []string{digestRef, "other@" + testImageID("c")},
			}})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:latest"},
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
			},
		},
		{
			name: "explicit digest lookup works without stored names", selector: digestRef,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoDigests: []string{digestRef},
			}})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: digestRef}},
		},
		{
			name: "synthesized digest resolves by target and repository even if not listed", selector: "myapp@" + idA,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoTags: []string{"myapp:old", "other:keep"}, RepoDigests: []string{"other@" + idA},
			}})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:old"}},
		},
		{
			name: "stored digest follows its repository tags", selector: digestRef,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", "myapp:latest", digestRef, "other:keep"),
				testImage(idB, "myapp@"+idB),
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:latest"},
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "dev1", ImageID: idA, Image: digestRef},
			},
		},
		{
			name: "digest pattern also expands matching repository tags", selector: "myapp@sha256:c*",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", "myapp:latest", digestRef, "other:keep"),
				testImage(idB, "myapp@"+idB),
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:latest"},
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "dev1", ImageID: idA, Image: digestRef},
			},
		},
		{
			name: "digest pattern matches stored names and ignores derived names", selector: "myapp@sha256:*",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", digestRef),
				{ID: idB, RepoTags: []string{"myapp:latest"}, RepoDigests: []string{"myapp@" + idB}},
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "dev1", ImageID: idA, Image: digestRef},
			},
		},
		{
			name: "last repository tag includes its stored digest", selector: "myapp:old",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", digestRef, "other:keep"),
			})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:old"},
				{MachineName: "dev1", ImageID: idA, Image: digestRef},
			},
		},
		{
			name: "last tag does not add a synthesized digest row", selector: "myapp:old",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoTags: []string{"myapp:old", "other:keep"}, RepoDigests: []string{digestRef},
			}})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:old"}},
		},
		{
			name: "remaining repository tag preserves its stored digest", selector: "myapp:old",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, "myapp:old", "myapp:latest", digestRef),
			})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: "myapp:old"}},
		},
		{
			name: "digest without repository tags remains executable", selector: digestRef,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
				testImage(idA, digestRef, "other:keep"),
			})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: digestRef}},
		},
		{
			name: "digest-only image keeps its named reference", selector: idA,
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoTags: []string{digestRef}, RepoDigests: []string{digestRef, testImageID("c")},
			}})},
			want: []removeImageOperation{{MachineName: "dev1", ImageID: idA, Image: digestRef}},
		},
		{
			name: "tag pattern selects only matching tags", selector: "myapp:1.[12]",
			inventory: []*machineImageInventory{testMachineInventory("dev1", []image.Summary{{
				ID: idA, RepoTags: []string{"myapp:1.3", "myapp:1.2", "myapp:1.1"}, RepoDigests: []string{digestRef},
			}})},
			want: []removeImageOperation{
				{MachineName: "dev1", ImageID: idA, Image: "myapp:1.1"},
				{MachineName: "dev1", ImageID: idA, Image: "myapp:1.2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planImageRemovalFromInventory([]string{tt.selector}, tt.inventory)
			require.NoError(t, err)

			require.Len(t, plan.GroupPlans, 1)
			assert.Equal(t, tt.selector, plan.GroupPlans[0].Image)

			var got []removeImageOperation
			for _, op := range plan.GroupPlans[0].Operations {
				got = append(got, *op)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlanImageRemovalFromInventory_OverlappingSelectors(t *testing.T) {
	inventory := []*machineImageInventory{
		testMachineInventory("dev1", []image.Summary{testImage(testImageID("a"), "myapp:old", "myapp:latest")}),
		testMachineInventory("machine2", []image.Summary{testImage(testImageID("b"), "myapp:test", "myapp:old")}),
	}

	plan, err := planImageRemovalFromInventory([]string{"myapp:old", "myapp", "myapp:*", testImageID("a"), "myapp:old"},
		inventory)
	require.NoError(t, err)

	assert.Equal(t, &imageRemovalPlan{GroupPlans: []*removeImageGroupPlan{
		{Image: "myapp:old", Operations: []*removeImageOperation{
			{MachineName: "dev1", ImageID: testImageID("a"), Image: "myapp:old"},
			{MachineName: "machine2", ImageID: testImageID("b"), Image: "myapp:old"},
		}},
		{Image: "myapp", Operations: []*removeImageOperation{
			{MachineName: "dev1", ImageID: testImageID("a"), Image: "myapp:latest"},
			{MachineName: "machine2", ImageID: testImageID("b"), Image: "myapp:test"},
		}},
	}}, plan, "the first selector owns each removal and empty groups are omitted")
}

func TestPlanImageRemovalFromInventory_DigestFollowsLastTagAcrossSelectors(t *testing.T) {
	imageID := testImageID("a")
	digestRef := "myapp@" + imageID
	inventory := []*machineImageInventory{testMachineInventory("dev1", []image.Summary{
		testImage(imageID, "myapp:old", "myapp:latest", "myapp:old", digestRef, digestRef, "other:keep"),
	})}

	plan, err := planImageRemovalFromInventory([]string{"myapp:old", "myapp:latest", digestRef}, inventory)
	require.NoError(t, err)

	assert.Equal(t, &imageRemovalPlan{GroupPlans: []*removeImageGroupPlan{
		{Image: "myapp:old", Operations: []*removeImageOperation{
			{MachineName: "dev1", ImageID: imageID, Image: "myapp:old"},
		}},
		{Image: "myapp:latest", Operations: []*removeImageOperation{
			{MachineName: "dev1", ImageID: imageID, Image: "myapp:latest"},
			{MachineName: "dev1", ImageID: imageID, Image: digestRef},
		}},
	}}, plan, "digest cleanup belongs to the last tag removal, so the later digest selector has no extra operation")
}

func TestPlanImageRemovalFromInventory_RejectsInvalidOrMissingSelectors(t *testing.T) {
	idA := "sha256:abcdef" + strings.Repeat("a", 58)
	idB := "sha256:abcdef" + strings.Repeat("b", 58)
	inventory := []*machineImageInventory{
		testMachineInventory("dev1", []image.Summary{{
			ID: idA, RepoTags: []string{"myapp:old"}, RepoDigests: []string{"myapp@" + idA, "derived@" + idA},
		}}),
		testMachineInventory("machine2", []image.Summary{{ID: idB, RepoTags: []string{"myapp:old"}}}),
	}

	tests := []struct {
		name   string
		images []string
		err    string
	}{
		{"missing repository", []string{"missing"}, "not found"},
		{"repository only has a derived digest", []string{"derived"}, "not found"},
		{"missing tag", []string{"myapp:missing"}, "not found"},
		{"missing pattern", []string{"other:*"}, "not found"},
		{"pattern only matches a derived digest", []string{"myapp@sha256:*"}, "not found"},
		{"missing ID", []string{testImageID("c")}, "not found"},
		{"ambiguous ID prefix", []string{"abcdef"}, "ambiguous"},
		{"invalid name", []string{"MyApp:old"}, "invalid image"},
		{"invalid pattern", []string{"myapp:["}, "invalid image pattern"},
		{"one missing selector rejects the whole plan", []string{"myapp:old", "myapp:missing"}, "not found"},
		{"one invalid selector rejects the whole plan", []string{"myapp:old", "myapp:["}, "invalid image pattern"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planImageRemovalFromInventory(tt.images, inventory)
			require.ErrorContains(t, err, tt.err)
			assert.Nil(t, plan, "a failed selector must not return a partial plan")
		})
	}

	t.Run("ID prefix resolves within the supplied machines", func(t *testing.T) {
		plan, err := planImageRemovalFromInventory([]string{"abcdef"}, inventory[:1])
		require.NoError(t, err)
		require.Len(t, plan.GroupPlans, 1)
		require.Len(t, plan.GroupPlans[0].Operations, 1)
		assert.Equal(t, idA, plan.GroupPlans[0].Operations[0].ImageID)
	})
}

func TestPlanImageRemovalFromInventory_InUseImagesAreStillPlanned(t *testing.T) {
	img := testImage(testImageID("a"), "myapp:old", "myapp:current")
	img.Containers = 1

	for _, selector := range []string{"myapp", "myapp:current", img.ID, "myapp:*"} {
		t.Run(selector, func(t *testing.T) {
			plan, err := planImageRemovalFromInventory([]string{selector}, []*machineImageInventory{
				testMachineInventory("dev1", []image.Summary{img}),
			})
			require.NoError(t, err)
			assert.False(t, plan.IsEmpty(), "usage protection belongs to the backend at execution time")
		})
	}
}

func testImageID(char string) string { return "sha256:" + strings.Repeat(char, 64) }

func testImage(imageID string, tags ...string) image.Summary {
	return image.Summary{ID: imageID, RepoTags: tags}
}

func testMachineInventory(name string, images []image.Summary) *machineImageInventory {
	return &machineImageInventory{machineName: name, images: images}
}
