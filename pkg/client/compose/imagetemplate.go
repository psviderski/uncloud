package compose

import (
	"bytes"
	"fmt"
	"text/template"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/psviderski/uncloud/internal/gitutil"
)

const (
	// DefaultTagTemplate is the default tag template appended to images without a tag when building locally.
	// Format:
	// If the project workdir is a git repo: "2025-08-16-130734.84d33bb" or "2025-08-16-130734.84d33bb.dirty"
	// If not a git repo: "2025-08-16-130734" which is the current date and time.
	DefaultTagTemplate = `{{if .Git.IsRepo}}{{gitdate "2006-01-02-150405"}}.{{gitsha 7}}{{if .Git.IsDirty}}.dirty{{end}}{{else}}{{date "2006-01-02-150405"}}{{end}}`
	// DefaultImageTemplate is the default image template used when no image is specified.
	DefaultImageTemplate = "{{.Project}}/{{.Service}}:" + DefaultTagTemplate
)

// ImageTemplateContext contains structured data for image template processing.
type ImageTemplateContext struct {
	// Project is the project name.
	Project string
	// Service is the service name where the image is defined.
	Service string
	// Git is the git repo state.
	Git gitutil.GitState
	// Tag is the rendered DefaultTagTemplate used for images without a tag.
	Tag string
}

// newImageTemplateContext creates an ImageTemplateContext for the given project, service, and git state.
func newImageTemplateContext(
	projectName, serviceName string, gitState gitutil.GitState,
) (ImageTemplateContext, error) {
	ctx := ImageTemplateContext{
		Project: projectName,
		Service: serviceName,
		Git:     gitState,
	}

	tag, err := processImageTemplate(DefaultTagTemplate, ctx)
	if err != nil {
		return ImageTemplateContext{}, fmt.Errorf("process default tag template: %w", err)
	}
	ctx.Tag = tag

	return ctx, nil
}

// ProcessImageTemplates processes image names in services to expand Go template expressions using
// git metadata. If a service has no image specified, uses the default template {{.Project}}/{{.Service}}:{{.Tag}}.
// If a service has a build section and an image without a tag, appends the default tag template.
// Fully specified images are left unchanged.
//
// Template context: ImageTemplateContext
// Template functions: gitsha(length?), gitdate(format, timezone?), date(format, timezone?)
//
// Examples:
//
//	image: myapp:{{.Tag}}                        → myapp:2025-08-16-130734.84d33bb
//	image: registry.io/api:v1.0-{{gitsha 7}}     → registry.io/api:v1.0-84d33bb
//	image: worker:{{gitdate "2006-01-02"}}       → worker:2025-08-16
//	build: . / image: mybackend                  → mybackend:2025-08-16-130734.84d33bb
//	build: . / (no image)                        → myproject/myservice:2025-08-16-130734.84d33bb
//	image: postgres                              → postgres (unchanged)
func ProcessImageTemplates(project *types.Project) (*types.Project, error) {
	gitState, err := gitutil.InspectGitState(project.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("inspect git state at '%s': %w", project.WorkingDir, err)
	}
	ctx, err := newImageTemplateContext(project.Name, "", gitState)
	if err != nil {
		return nil, err
	}

	// Process each service's image.
	for name, service := range project.Services {
		ctx.Service = service.Name

		processedImage := ""
		image := service.Image

		if image == "" {
			image = DefaultImageTemplate
		} else {
			// Check if image contains any template markers.
			processed, err := processImageTemplate(image, ctx)
			if err != nil {
				return nil, fmt.Errorf("process image template for service '%s': %w", name, err)
			}

			if processed == image {
				// Image doesn't contain template markers.
				// If it doesn't have a tag and has a build section, use the default template for the tag.
				tagged, err := hasTag(image)
				if err != nil {
					return nil, err
				}
				if !tagged && service.Build != nil {
					image = image + ":" + DefaultTagTemplate
				}
			} else {
				// Image contains a template which has been processed successfully so use the result.
				processedImage = processed
			}
		}

		if processedImage == "" {
			if processedImage, err = processImageTemplate(image, ctx); err != nil {
				return nil, fmt.Errorf("process image template for service '%s': %w", name, err)
			}
		}

		service.Image = processedImage
		project.Services[name] = service
	}

	return project, nil
}

// processImageTemplate processes a single image string through the template engine.
func processImageTemplate(image string, ctx ImageTemplateContext) (string, error) {
	funcMap := template.FuncMap{
		"gitsha": func(length ...int) string {
			l := 0
			if len(length) > 0 {
				l = length[0]
			}
			return ctx.Git.ShortSHA(l)
		},
		"gitdate": func(layout string, timezone ...string) string {
			if !ctx.Git.IsRepo || ctx.Git.Date.IsZero() {
				return ""
			}

			date := ctx.Git.Date
			if len(timezone) > 0 {
				if loc, err := time.LoadLocation(timezone[0]); err == nil {
					date = date.In(loc)
				}
			}

			return date.Format(layout)
		},
		"date": func(layout string, timezone ...string) string {
			t := time.Now().UTC()
			if len(timezone) > 0 {
				if loc, err := time.LoadLocation(timezone[0]); err == nil {
					t = t.In(loc)
				}
			}

			return t.Format(layout)
		},
	}

	// Parse and execute the template.
	tmpl, err := template.New("image").Funcs(funcMap).Parse(image)
	if err != nil {
		return "", fmt.Errorf("parse image template '%s': %w", image, err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute image template: %w", err)
	}

	return buf.String(), nil
}

// hasTag checks if an image name already has an explicit tag or digest.
// Returns an error if the image reference is invalid.
func hasTag(image string) (bool, error) {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return false, fmt.Errorf("parse image reference '%s': %w", image, err)
	}

	// Check if it has an explicit tag (e.g., "myapp:v1.0").
	if _, ok := ref.(reference.Tagged); ok {
		return true, nil
	}
	// Check if it has a digest (e.g., "myapp@sha256:...").
	if _, ok := ref.(reference.Digested); ok {
		return true, nil
	}

	return false, nil
}
