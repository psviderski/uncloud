# Image tag template

Template syntax for tagging built images.

## Overview

When building service images as part of [`uc build`](../9-cli-reference/uc_build.md) or
[`uc deploy`](../9-cli-reference/uc_deploy.md) commands, Uncloud automatically generates image tags based on the current
Git repository state. You can customise the image name and tag format for the built images using the
[Go template](https://pkg.go.dev/text/template) syntax and environment variables.

## Default template

If you **don't specify** an `image` attribute for a service with a `build` section, Uncloud uses the following Go
template for tagging the built image:

```
{{.Project}}/{{.Service}}:{{if .Git.IsRepo}}{{gitdate "2006-01-02-150405"}}.{{gitsha 7}}{{if .Git.IsDirty}}.dirty{{end}}{{else}}{{date "2006-01-02-150405"}}{{end}}
```

```yaml title="compose.yaml"
services:
  web:
    build: .
```

This generates image tags as follows:

- Git repository (clean): `myapp/web:2025-10-30-223604.84d33bb`
- Git repository (with uncommitted changes): `myapp/web:2025-10-30-223604.84d33bb.dirty`
- Non-Git directory: `myapp/web:2025-10-31-120651`

If you specify only an **image name without a tag** in the `image` attribute, Uncloud appends the tag portion of the
default template to your image name.

```yaml title="compose.yaml"
services:
  web:
    build: .
    image: webapp   # → webapp:2025-10-30-223604.84d33bb
```

If you specify a full **image name with tag** in the `image` attribute, Uncloud uses it as-is without modification.

```yaml title="compose.yaml"
services:
  web:
    build: .
    image: webapp:1.2.3   # → webapp:1.2.3
```

## Template functions

### `gitsha [length]`

Returns the Git commit SHA, optionally truncated to the specified length.

```yaml
image: myapp:{{gitsha 7}}   # → myapp:84d33bb
image: myapp:{{gitsha}}     # → myapp:84d33bbf0dbb37f96e7df6a5010aed7bab00b089
```

Returns empty string if the working directory is not a Git repository.

### `gitdate "format" ["timezone"]`

Returns the current Git commit date/time formatted using [Go time layout format](#date-format-reference). The `timezone`
parameter is optional and defaults to UTC.
Use [IANA timezone names](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones) like `America/New_York` or
`Europe/London`.

```yaml
image: myapp:{{gitdate "2006-01-02"}}                               # → myapp:2025-10-30
image: myapp:{{gitdate "20060102-150405"}}                          # → myapp:20251030-223604
image: myapp:{{gitdate "2006-01-02-150405" "Australia/Brisbane"}}   # → myapp:2025-10-31-083604
```

Returns empty string if the working directory is not a Git repository.

### `date "format" ["timezone"]`

Returns the current local date/time formatted using [Go time layout format](#date-format-reference). The `timezone`
parameter is optional and defaults to UTC.
Use [IANA timezone names](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones) like `America/New_York` or
`Europe/London`.

```yaml
image: myapp:{{date "2006-01-02"}}                # → myapp:2025-10-31
image: myapp:{{date "20060102-150405"}}           # → myapp:20251031-120651
image: myapp:{{date "20060102-150405" "Local"}}   # → myapp:20251031-220651
```

### Date format reference

Go uses a reference time `Mon Jan 2 15:04:05 MST 2006` for formatting. Replace reference components with desired format:

| Component | Reference | Example     |
|-----------|-----------|-------------|
| Year      | `2006`    | `2025`      |
| Month     | `01`      | `10`        |
| Day       | `02`      | `30`        |
| Hour      | `15`      | `22` (24hr) |
| Minute    | `04`      | `36`        |
| Second    | `05`      | `04`        |

**Common patterns:**

| Format                 | Pattern             | Example             |
|------------------------|---------------------|---------------------|
| ISO 8601 date          | `2006-01-02`        | `2025-10-30`        |
| Compact date           | `20060102`          | `20251030`          |
| Date with compact time | `2006-01-02-150405` | `2025-10-30-223604`

See Go [time.Format documentation](https://pkg.go.dev/time#Time.Format) for all formatting options.

## Template fields

Access metadata about your project, service, and Git state:

| Field          | Type      | Description                                                        | Example                     |
|----------------|-----------|--------------------------------------------------------------------|-----------------------------|
| `.Project`     | string    | Project name from `name` in Compose file or working directory name | `myapp`                     |
| `.Service`     | string    | Service name                                                       | `web`                       |
| `.Tag`         | string    | Pre-rendered default tag (without image name)                      | `2025-10-30-223604.84d33bb` |
| `.Git.IsRepo`  | bool      | Whether working directory is a Git repository                      | `true` or `false`           |
| `.Git.IsDirty` | bool      | Whether there are uncommitted changes                              | `true` or `false`           |
| `.Git.SHA`     | string    | Full SHA (40 characters) of the latest Git commit                  | `84d33bb1234567...`         |
| `.Git.Date`    | time.Time | Git commit date/time (use `gitdate` function to format)            | -                           |

## Environment variable interpolation

Combine templates with environment variable
[interpolation](https://github.com/compose-spec/compose-spec/blob/main/spec.md#interpolation) using Bash-like syntax.
The environment variables are expanded before rendering the template.

```yaml
# CI build number from environment
image: myapp:{{gitdate "20060102"}}.{{gitsha 7}}.${GITHUB_RUN_ID}   # → myapp:20251030.84d33bb.1234

# With default value
image: myapp:{{gitsha 7}}.${GITHUB_RUN_ID:-local}   # GITHUB_RUN_ID not set → myapp:84d33bb.local
```

## See also

- [Deploy an app](../4-guides/1-deployments/1-deploy-app.md): Deploy from source code or prebuilt images
- [Compose Build Specification](https://github.com/compose-spec/compose-spec/blob/main/build.md)
- [Compose Specification: image](https://github.com/compose-spec/compose-spec/blob/main/spec.md#image)
- [Go template documentation](https://pkg.go.dev/text/template)
- [Go Time.Format documentation](https://pkg.go.dev/time#Time.Format)
