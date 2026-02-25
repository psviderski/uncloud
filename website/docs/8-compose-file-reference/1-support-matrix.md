# Compose support matrix

Uncloud supports a subset of the [Compose specification](https://compose-spec.io/) with some extensions and limitations.
The following table shows the support status for main Compose features:

| Feature            | Support Status     | Notes                                                                                                                                 |
|--------------------|--------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| **Services**       |                    |                                                                                                                                       |
| `build`            | ✅ Supported        | Build context and Dockerfile                                                                                                          |
| `cap_add`          | ✅ Supported        | Additional kernel [capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)                                           |
| `cap_drop`         | ✅ Supported        | Which kernel [capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to drop                                        |
| `command`          | ✅ Supported        | Override container command                                                                                                            |
| `configs`          | ✅ Supported        | File-based and inline configs                                                                                                         |
| `cpus`             | ✅ Supported        | CPU limit                                                                                                                             |
| `depends_on`       | ⚠️ Limited         | Services deployed in order but conditions not checked                                                                                 |
| `devices`          | ✅ Supported        | Device mappings                                                                                                                       |
| `dns`              | ❌ Not supported    | Built-in service discovery                                                                                                            |
| `dns_search`       | ❌ Not supported    | Built-in service discovery                                                                                                            |
| `entrypoint`       | ✅ Supported        | Override container entrypoint                                                                                                         |
| `env_file`         | ✅ Supported        | Environment file                                                                                                                      |
| `environment`      | ✅ Supported        | Environment variables                                                                                                                 |
| `gpus`             | ✅ Supported        | GPU device access                                                                                                                     |
| `healthcheck`      | ✅ Supported        | Health check configuration                                                                                                            |
| `image`            | ✅ Supported        | Container image specification                                                                                                         |
| `init`             | ✅ Supported        | Run init process in container                                                                                                         |
| `labels`           | ❌ Not supported    |                                                                                                                                       |
| `links`            | ❌ Not supported    | Use service names for communication                                                                                                   |
| `logging`          | ✅ Supported        | Defaults to [local](https://docs.docker.com/engine/logging/drivers/local/) log driver                                                 |
| `mem_limit`        | ✅ Supported        | Memory limit                                                                                                                          |
| `mem_reservation`  | ✅ Supported        | Memory reservation                                                                                                                    |
| `mem_swappiness`   | ❌ Not supported    |                                                                                                                                       |
| `memswap_limit`    | ❌ Not supported    |                                                                                                                                       |
| `networks`         | ❌ Not supported    | All containers share cluster network                                                                                                  |
| `ports`            | ⚠️ Limited         | `mode: host` only, use [`x-ports`](#x-ports) for HTTP/HTTPS                                                                           |
| `privileged`       | ✅ Supported        | Run containers in privileged mode                                                                                                     |
| `pull_policy`      | ✅ Supported        | `always`, `missing`, `never`                                                                                                          |
| `secrets`          | ❌ Not supported    | Use configs or environment variables                                                                                                  |
| `security_opt`     | ❌ Not supported    |                                                                                                                                       |
| `storage_opt`      | ❌ Not supported    |                                                                                                                                       |
| `sysctls`          | ✅ Supported        | Namespaced kernel parameters                                                                                                          |
| `user`             | ✅ Supported        | Set container user                                                                                                                    |
| `volumes`          | ✅ Supported        | Named volumes, bind mounts, tmpfs                                                                                                     |
| **Deploy**         |                    |                                                                                                                                       |
| `labels`           | ❌ Not supported    |                                                                                                                                       |
| `mode`             | ✅ Supported        | Either `global` or `replicated`                                                                                                       |
| `placement`        | ❌ Not supported    | Use [`x-machines`](#x-machines) extension                                                                                             |
| `replicas`         | ✅ Supported        | Number of container replicas                                                                                                          |
| `resources`        | ⚠️ Limited         | CPU, memory limits and device reservations                                                                                            |
| `restart_policy`   | ❌ Not supported    | Defaults to `unless-stopped`                                                                                                          |
| `rollback_config`  | ❌ Not supported    | See [#151](https://github.com/psviderski/uncloud/issues/151)                                                                          |
| `update_config`    | ⚠️ Limited         | Only `order` supported (defaults to `start-first`). See [deployment strategies](../4-guides/1-deployments/4-deployment-strategies.md) |
| **Volumes**        |                    |                                                                                                                                       |
| Named volumes      | ✅ Supported        | Docker volumes                                                                                                                        |
| Bind mounts        | ✅ Supported        | Host path binding                                                                                                                     |
| Tmpfs mounts       | ✅ Supported        | In-memory filesystems                                                                                                                 |
| Volume labels      | ✅ Supported        | Custom labels                                                                                                                         |
| External volumes   | ✅ Supported        | Must exist before deployment                                                                                                          |
| Volume drivers     | ⚠️ Limited         | Local driver only                                                                                                                     |
| **Configs**        |                    |                                                                                                                                       |
| File-based configs | ✅ Supported        | Read from file                                                                                                                        |
| Inline configs     | ✅ Supported        | Defined in compose file                                                                                                               |
| External configs   | ❌ Not supported    | Not supported                                                                                                                         |
| Short syntax       | ❌ Not supported    | Use long syntax only                                                                                                                  |
| **Extensions**     |                    |                                                                                                                                       |
| `x-caddy`          | ✅ Uncloud-specific | Custom Caddy configuration                                                                                                            |
| `x-machines`       | ✅ Uncloud-specific | Machine placement constraints                                                                                                         |
| `x-ports`          | ✅ Uncloud-specific | Service port publishing                                                                                                               |

### Legend

- ✅ **Supported**: Feature works as documented
- ⚠️ **Limited**: Partial support or with restrictions
- ❌ **Not supported**: Feature is not (yet) available

## Uncloud extensions

Uncloud provides several custom extensions to enhance the Compose experience:

### `x-ports`

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

See [Publishing services](../3-concepts/1-ingress/2-publishing-services.md) for more details.

### `x-caddy`

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

See [Publishing services](../3-concepts/1-ingress/2-publishing-services.md) for more details.

### `x-machines`

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
