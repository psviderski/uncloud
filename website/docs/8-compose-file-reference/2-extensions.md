---
title: Compose extensions
---

# Uncloud-specific Compose extensions

Uncloud provides several custom extensions to the standard [Compose specification](https://compose-spec.io/) that let
you configure Uncloud-specific features directly in your Compose file.

See the [support matrix](1-support-matrix.md) for the full list of supported standard Compose features.

## `x-context`

Set the cluster context for all commands that use the Compose file, such as `deploy`, `build`, and `logs`. This is
useful when you manage multiple clusters and want to make sure a Compose file is always deployed to the right one. No
need to remember to manually switch clusters with `uc ctx` or `--context`.

`x-context` is a top-level key, not a service-level attribute.

```yaml
x-context: prod

services:
  web:
    image: nginx
```

The `--context` and `--connect` flags take precedence over `x-context`. If you don't specify any of these, the current
context from your Uncloud config (`--uncloud-config`) is used.

:::warning

If you share the Compose file with other users, make sure to use the same context name for the target cluster in your
Uncloud configs.

:::

## `x-ports`

Expose HTTP/HTTPS service ports via the Caddy reverse proxy, or bind TCP/UDP ports directly to the host:

```yaml
services:
  web:
    image: nginx
    x-ports:
      - 80/https
      - example.com:80/https
      - 8080:80/tcp@host
```

See [Publishing services](../3-concepts/2-ingress/2-publishing-services.md) for more details.

## `x-caddy`

Custom Caddy reverse proxy configuration for a service:

```yaml
services:
  web:
    image: nginx
    x-caddy: |
      example.com {
        reverse_proxy {{upstreams 80}}
      }
```

See [Publishing services](../3-concepts/2-ingress/2-publishing-services.md) for more details.

## `x-machines`

Restrict which machines can run your service. If you deploy multiple replicas, Uncloud automatically spreads them across
the specified machines.

```yaml
services:
  web:
    image: nginx
    x-machines:
      - machine-1
      - machine-2
    # Short syntax for a single machine
    # x-machines: machine-1
```

## `x-pre_deploy`

Configure a pre-deploy hook to run a one-off command in a separate container and wait for it to finish successfully
before rolling out service containers. It's useful for preparation tasks that need to run before every service
deployment, such as database migrations, static asset uploads, or cache invalidation.

The hook container uses the service's image and inherits its environment variables, volumes, placement, and compute
resources. If the command fails or times out (5 minutes by default), the deployment stops.

```yaml
services:
  web:
    build: .
    x-pre_deploy:
      command: python manage.py migrate
      environment:
        LOG_LEVEL: DEBUG
      timeout: 10m
```

### Attributes

| Attribute     | Type                    | Default         | Description                                                                                                                                                        |
|---------------|-------------------------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `command`     | string / list           | (required)      | The command to run in the hook container (same format as the service's [`command`](https://github.com/compose-spec/compose-spec/blob/main/05-services.md#command)) |
| `environment` | map / list of KEY=VALUE | -               | Additional env vars that override or extend the service's [`environment`](https://github.com/compose-spec/compose-spec/blob/main/05-services.md#environment)       |
| `privileged`  | bool                    | service's value | Override the service's [`privileged`](https://github.com/compose-spec/compose-spec/blob/main/05-services.md#privileged) mode                                       |
| `timeout`     | duration                | `5m`            | Max time to wait for the command to finish before killing it (e.g., `1m30s`, `30m`, `1h`)                                                                          |
| `user`        | string                  | service's value | Override the service's [`user`](https://github.com/compose-spec/compose-spec/blob/main/05-services.md#user) to run as (`user`, `UID`, `user:group`, or `UID:GID`)  |

The hook container also gets `UNCLOUD_HOOK_PRE_DEPLOY=true` environment variable set automatically.

See [Pre-deploy hooks](../4-guides/1-deployments/5-pre-deploy-hooks.md) for more details, usage examples, and failure
handling.
