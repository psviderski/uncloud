# Compose support matrix

Uncloud supports a subset of the [Compose specification](https://compose-spec.io/) with some extensions and limitations.
The following table shows the support status for main Compose features:

| Feature            | Support Status      | Notes                                               |
| ------------------ | ------------------- | --------------------------------------------------- |
| **Services**       |                     |                                                     |
| `build`            | ✅ Supported        | Build context and Dockerfile                        |
| `command`          | ✅ Supported        | Override container command                          |
| `configs`          | ✅ Supported        | File-based and inline configs                       |
| `cpus`             | ✅ Supported        | CPU limit                                           |
| `depends_on`       | ❌ Not supported    | Services start independently                        |
| `dns`              | ❌ Not supported    | Built-in service discovery                          |
| `dns_search`       | ❌ Not supported    | Built-in service discovery                          |
| `entrypoint`       | ✅ Supported        | Override container entrypoint                       |
| `env_file`         | ✅ Supported        | Environment file                                    |
| `environment`      | ✅ Supported        | Environment variables                               |
| `image`            | ✅ Supported        | Container image specification                       |
| `init`             | ✅ Supported        | Run init process in container                       |
| `labels`           | ❌ Not supported    | Not currently needed                                |
| `links`            | ❌ Not supported    | Use service names for communication                 |
| `logging`          | ✅ Supported        | Uses Docker daemon logging                          |
| `mem_limit`        | ✅ Supported        | Memory limit                                        |
| `mem_reservation`  | ✅ Supported        | Memory reservation                                  |
| `mem_swappiness`   | ❌ Not supported    |                                                     |
| `memswap_limit`    | ❌ Not supported    |                                                     |
| `networks`         | ❌ Not supported    | All containers share cluster network                |
| `ports`            | ⚠️ Limited          | Basic port publishing. Use `x-ports` for HTTP/HTTPS |
| `privileged`       | ✅ Supported        | Run containers in privileged mode                   |
| `pull_policy`      | ✅ Supported        | always, missing, never                              |
| `secrets`          | ❌ Not supported    | Use configs or environment variables                |
| `security_opt`     | ❌ Not supported    |                                                     |
| `storage_opt`      | ❌ Not supported    |                                                     |
| `user`             | ✅ Supported        | Set container user                                  |
| `volumes`          | ✅ Supported        | Named volumes, bind mounts, tmpfs                   |
| **Deploy**         |                     |                                                     |
| `labels`           | ❌ Not supported    | Not needed                                          |
| `mode`             | ⚠️ Limited          | Only `replicated` supported                         |
| `placement`        | ❌ Not supported    | Use `x-machines` extension                          |
| `replicas`         | ✅ Supported        | Number of container replicas                        |
| `resources`        | ⚠️ Limited          | CPU and memory limits only                          |
| `restart_policy`   | ❌ Not supported    | Services auto-restart                               |
| **Volumes**        |                     |                                                     |
| Named volumes      | ✅ Supported        | Docker volumes                                      |
| Bind mounts        | ✅ Supported        | Host path binding                                   |
| Tmpfs mounts       | ✅ Supported        | In-memory filesystems                               |
| Volume labels      | ✅ Supported        | Custom labels                                       |
| External volumes   | ⚠️ Limited          | Must exist before deployment                        |
| Volume drivers     | ⚠️ Limited          | Local driver only                                   |
| **Configs**        |                     |                                                     |
| File-based configs | ✅ Supported        | Read from file                                      |
| Inline configs     | ✅ Supported        | Defined in compose file                             |
| External configs   | ❌ Not supported    | Not supported                                       |
| Short syntax       | ❌ Not supported    | Use long syntax only                                |
| **Extensions**     |                     |                                                     |
| `x-caddy`          | ✅ Uncloud-specific | Custom Caddy configuration                          |
| `x-machines`       | ✅ Uncloud-specific | Machine placement constraints                       |
| `x-ports`          | ✅ Uncloud-specific | HTTP/HTTPS port publishing                          |

### Legend

- ✅ **Supported**: Feature works as documented
- ⚠️ **Limited**: Partial support or with restrictions
- ❌ **Not supported**: Feature is not (yet) available

## Uncloud Extensions

Uncloud provides several custom extensions to enhance the Compose experience:

### `x-ports`

Define HTTP/HTTPS endpoints for services:

```yaml
services:
  web:
    image: nginx
    x-ports:
      - 80/https
      - example.com:80/https
```

### `x-caddy`

Custom Caddy reverse proxy configuration:

```yaml
services:
  web:
    image: nginx
    x-caddy: |
      example.com {
        reverse_proxy {{ upstreams 80 }}
      }
```

### `x-machines`

Specify on what machines the service should run:

```yaml
services:
  web:
    image: nginx
    x-machines: ["web-1", "web-2"]
```
