# Runtime templates

Runtime templates let service configuration use metadata that is only known when Uncloud creates a container. They use
[Go template](https://pkg.go.dev/text/template) syntax.

Uncloud renders runtime templates on the destination machine before it creates each container. This means that every
replica can receive a value based on its own container metadata.

## Available metadata

Runtime templates currently expose the following fields:

| Field             | Description                    | Example    |
|-------------------|--------------------------------|------------|
| `.Container.Name` | Name assigned to the container | `app-c1zd` |

## Supported template locations

You can currently use runtime templates in these locations of a service definition:

| Location                        | Compose attribute                                                           |
|---------------------------------|-----------------------------------------------------------------------------|
| Host path of a bind mount       | `volumes[].source` or `volumes` short-syntax `- /host/path:/container/path` |
| Mount path inside the container | `volumes[].target` or `volumes` short-syntax `- /host/path:/container/path` |

:::tip Want other locations or metadata fields?

If you want to use runtime templates in other locations, such as environment variable, or use other metadata fields, add
a 👍 reaction or comment with your use case on the issue [#431](https://github.com/psviderski/uncloud/issues/431).

:::

## Use runtime templates in a Compose file

The following service gets a separate host path for every replica:

```yaml title="compose.yaml"
services:
  app:
    image: app:latest
    volumes:
      - "/var/lib/app/{{.Container.Name}}:/data"
    scale: 2
```

If Uncloud names the containers `app-c1zd` and `app-f7kx`, their host paths are `/var/lib/app/app-c1zd` and
`/var/lib/app/app-f7kx`.

You can combine runtime templates with
[Compose environment interpolation](https://github.com/compose-spec/compose-spec/blob/main/12-interpolation.md):

```yaml
volumes:
  - "${DATA_ROOT:-/var/lib/app}/{{.Container.Name}}:/data"
```

Uncloud expands the `DATA_ROOT` environment variable on the local machine first and renders `.Container.Name` later on
the destination machine.

## Use runtime templates with `uc run`

Quote the volume argument so your shell passes the template through unchanged:

```shell
uc run --replicas 2 \
  --volume "/var/lib/app/{{.Container.Name}}:/data" \
  app:latest
```

:::warning Clean up templated host directories

Uncloud does not remove host directories created for bind mounts when it replaces a container or removes a service. If a
runtime template gives each container a unique host directory, you are responsible for cleaning up directories that are
no longer needed.

If you only need per-container ephemeral storage, consider a `tmpfs` mount. An anonymous volume declared by the Docker
image can also provide Docker-managed disk storage for each container. Use a named volume or a stable bind mount path
for data that must remain available across container replacements.

:::

## Runtime templates and image tag templates

Runtime templates are separate from [image tag templates](../../8-compose-file-reference/3-image-tag-template.md). Image
tag templates run locally when Uncloud builds an image. Runtime templates run on a cluster machine when Uncloud deploys
a container. Each template type exposes different metadata.

## See also

- [Compose support matrix](../../8-compose-file-reference/1-support-matrix.md)
- [`uc run` reference](../../9-cli-reference/uc_run.md)
