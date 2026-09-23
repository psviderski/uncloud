package api

import (
	"bytes"
	"fmt"
	"text/template"
)

// RuntimeTemplateContext contains the data available to runtime templates in a service spec.
type RuntimeTemplateContext struct {
	Container RuntimeTemplateContainerContext
}

// RuntimeTemplateContainerContext contains container data available to runtime templates in a service spec.
type RuntimeTemplateContainerContext struct {
	Name string
}

// RenderRuntimeTemplates returns a copy of the service spec with supported runtime template expressions rendered
// using metadata from ctx. Runtime templates use Go template expressions such as {{.Container.Name}} and are supported
// in bind volume host paths and volume mount container paths. The original service spec is not modified.
func (s *ServiceSpec) RenderRuntimeTemplates(ctx RuntimeTemplateContext) (ServiceSpec, error) {
	if ctx.Container.Name == "" {
		return ServiceSpec{}, fmt.Errorf("container name must not be empty")
	}

	spec := s.Clone()

	for i := range spec.Volumes {
		volume := &spec.Volumes[i]
		if volume.Type != VolumeTypeBind {
			continue
		}
		if err := volume.Validate(); err != nil {
			return ServiceSpec{}, fmt.Errorf("invalid bind volume '%s': %w", volume.Name, err)
		}

		hostPath, err := renderRuntimeTemplate(volume.BindOptions.HostPath, ctx)
		if err != nil {
			return ServiceSpec{}, fmt.Errorf("render runtime template in bind volume '%s' host path: %w",
				volume.Name, err)
		}
		volume.BindOptions.HostPath = hostPath

		if err = volume.Validate(); err != nil {
			return ServiceSpec{}, fmt.Errorf("invalid rendered bind volume '%s': %w", volume.Name, err)
		}
	}

	for i := range spec.Container.VolumeMounts {
		volumeMount := &spec.Container.VolumeMounts[i]
		if err := volumeMount.Validate(); err != nil {
			return ServiceSpec{}, fmt.Errorf("invalid volume mount '%s': %w", volumeMount.VolumeName, err)
		}

		containerPath, err := renderRuntimeTemplate(volumeMount.ContainerPath, ctx)
		if err != nil {
			return ServiceSpec{}, fmt.Errorf(
				"render runtime template in volume '%s' container path: %w", volumeMount.VolumeName, err)
		}
		volumeMount.ContainerPath = containerPath

		if err = volumeMount.Validate(); err != nil {
			return ServiceSpec{}, fmt.Errorf("invalid rendered volume mount '%s': %w", volumeMount.VolumeName, err)
		}
	}

	return spec, nil
}

func renderRuntimeTemplate(value string, ctx RuntimeTemplateContext) (string, error) {
	tmpl, err := template.New("runtime template").Option("missingkey=error").Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse runtime template: %w", err)
	}

	var rendered bytes.Buffer
	if err = tmpl.Execute(&rendered, ctx); err != nil {
		return "", fmt.Errorf("execute runtime template: %w", err)
	}

	return rendered.String(), nil
}

func validateRuntimeTemplates(spec *ServiceSpec) error {
	_, err := spec.RenderRuntimeTemplates(RuntimeTemplateContext{
		Container: RuntimeTemplateContainerContext{Name: "validation-container-name"},
	})
	return err
}
