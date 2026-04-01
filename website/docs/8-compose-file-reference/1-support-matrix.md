# Compose support matrix

Uncloud supports a subset of the [Compose specification](https://compose-spec.io/) with some extensions and limitations.
The following table shows the support status for main Compose features.

:::info

If you rely on a specific Compose feature that is not supported by Uncloud, please submit a feature request in
[GitHub Discussions](https://github.com/psviderski/uncloud/discussions) or on [Discord](https://uncloud.run/discord).

:::

| Feature                          | Support Status      | Notes                                                                                                          |
| -------------------------------- | ------------------- | -------------------------------------------------------------------------------------------------------------- |
| **Services**                     |                     |                                                                                                                |
| `build`                          | ✅ Supported        | Build context and Dockerfile                                                                                   |
| `cap_add`                        | ✅ Supported        | Additional kernel [capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)                    |
| `cap_drop`                       | ✅ Supported        | Which kernel [capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to drop                 |
| `command`                        | ✅ Supported        | Override container command                                                                                     |
| `configs`                        | ✅ Supported        | File-based and inline configs                                                                                  |
| `cpus`                           | ✅ Supported        | CPU limit                                                                                                      |
| `depends_on`                     | ⚠️ Limited          | Services deployed in order but conditions not checked                                                          |
| `devices`                        | ✅ Supported        | Device mappings                                                                                                |
| `dns`                            | ❌ Not supported    | Built-in service discovery                                                                                     |
| `dns_search`                     | ❌ Not supported    | Built-in service discovery                                                                                     |
| `entrypoint`                     | ✅ Supported        | Override container entrypoint                                                                                  |
| `env_file`                       | ✅ Supported        | Environment file                                                                                               |
| `environment`                    | ✅ Supported        | Environment variables                                                                                          |
| `gpus`                           | ✅ Supported        | GPU device access                                                                                              |
| `healthcheck`                    | ✅ Supported        | Health check configuration                                                                                     |
| `image`                          | ✅ Supported        | Container image specification                                                                                  |
| `init`                           | ✅ Supported        | Run init process in container                                                                                  |
| `labels`                         | ❌ Not supported    |                                                                                                                |
| `links`                          | ❌ Not supported    | Use service names for communication                                                                            |
| `logging`                        | ✅ Supported        | Defaults to [local](https://docs.docker.com/engine/logging/drivers/local/) log driver                          |
| `mem_limit`                      | ✅ Supported        | Memory limit                                                                                                   |
| `mem_reservation`                | ✅ Supported        | Memory reservation                                                                                             |
| `mem_swappiness`                 | ❌ Not supported    |                                                                                                                |
| `memswap_limit`                  | ❌ Not supported    |                                                                                                                |
| `networks`                       | ❌ Not supported    | All containers share cluster network                                                                           |
| `pid`                            | ✅ Supported        | Set the PID namespace mode, `pid: host` only                                                                   |
| `ports`                          | ⚠️ Limited          | `mode: host` only, use [`x-ports`](#x-ports) for HTTP/HTTPS                                                    |
| `privileged`                     | ✅ Supported        | Run containers in privileged mode                                                                              |
| `pull_policy`                    | ✅ Supported        | `always`, `missing`, `never`                                                                                   |
| `secrets`                        | ❌ Not supported    | Use configs or environment variables                                                                           |
| `security_opt`                   | ❌ Not supported    |                                                                                                                |
| `shm_size`                       | ✅ Supported        | Shared memory size                                                                                             |
| `stop_grace_period`              | ✅ Supported        | Time to wait after SIGTERM before SIGKILL                                                                      |
| `storage_opt`                    | ❌ Not supported    |                                                                                                                |
| `sysctls`                        | ✅ Supported        | Namespaced kernel parameters                                                                                   |
| `ulimits`                        | ✅ Supported        | Resource limits                                                                                                |
| `user`                           | ✅ Supported        | Set container user                                                                                             |
| `volumes`                        | ✅ Supported        | Named volumes, bind mounts, tmpfs                                                                              |
| **Deploy**                       |                     |                                                                                                                |
| `labels`                         | ❌ Not supported    |                                                                                                                |
| `mode`                           | ✅ Supported        | Either `global` or `replicated`                                                                                |
| `placement`                      | ❌ Not supported    | Use [`x-machines`](#x-machines) extension                                                                      |
| `replicas`                       | ✅ Supported        | Number of container replicas                                                                                   |
| `resources`                      | ⚠️ Limited          | CPU, memory limits and device reservations                                                                     |
| `restart_policy`                 | ❌ Not supported    | Defaults to `unless-stopped`                                                                                   |
| `rollback_config`                | ❌ Not supported    | See [#151](https://github.com/psviderski/uncloud/issues/151)                                                   |
| `update_config`                  | ⚠️ Limited          | `order` and `monitor` supported. See [rolling deployments](../4-guides/1-deployments/4-rolling-deployments.md) |
| **Volumes**                      |                     |                                                                                                                |
| Named volumes                    | ✅ Supported        | Docker volumes                                                                                                 |
| Bind mounts                      | ✅ Supported        | Host path binding                                                                                              |
| Tmpfs mounts                     | ✅ Supported        | In-memory filesystems                                                                                          |
| Volume labels                    | ✅ Supported        | Custom labels                                                                                                  |
| External volumes                 | ✅ Supported        | Must exist before deployment                                                                                   |
| [Volume drivers][volume-drivers] | ✅ Supported        | `local` (supports [NFS][volume-nfs], [CIFS/Samba][volume-cifs]) and manually installed third-party drivers     |
| **Configs**                      |                     |                                                                                                                |
| File-based configs               | ✅ Supported        | Read from file                                                                                                 |
| Inline configs                   | ✅ Supported        | Defined in compose file                                                                                        |
| External configs                 | ❌ Not supported    | Not supported                                                                                                  |
| Short syntax                     | ❌ Not supported    | Use long syntax only                                                                                           |
| **Extensions**                   |                     |                                                                                                                |
| `x-context`                      | ✅ Uncloud-specific | Cluster context override                                                                                       |
| `x-caddy`                        | ✅ Uncloud-specific | Custom Caddy configuration                                                                                     |
| `x-machines`                     | ✅ Uncloud-specific | Machine placement constraints                                                                                  |
| `x-ports`                        | ✅ Uncloud-specific | Service port publishing                                                                                        |

[volume-drivers]: https://docs.docker.com/engine/storage/volumes/#use-a-volume-driver
[volume-nfs]: https://docs.docker.com/engine/storage/volumes/#create-a-service-which-creates-an-nfs-volume
[volume-cifs]: https://docs.docker.com/engine/storage/volumes/#create-cifssamba-volumes

### Legend

- ✅ **Supported**: Feature works as documented
- ⚠️ **Limited**: Partial support or with restrictions
- ❌ **Not supported**: Feature is not (yet) available

## Uncloud extensions

Uncloud provides several custom extensions to enhance the Compose experience:

### `x-context`

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

See [Publishing services](../3-concepts/2-ingress/2-publishing-services.md) for more details.

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

See [Publishing services](../3-concepts/2-ingress/2-publishing-services.md) for more details.

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

### `x-registry`

This allows for setting credentails for private Docker registries, and can hold multiple registries.

```yaml
services:
  web:
    image: myregistry.example.com/private/nginx
    x-registry:
      myregistry.example.com:
        username: xxxxx
        password: yyyyy
```
