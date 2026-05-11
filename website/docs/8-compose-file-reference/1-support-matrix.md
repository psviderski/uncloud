# Compose support matrix

Uncloud supports a subset of the [Compose specification](https://compose-spec.io/) with some
[extensions](2-extensions.md) and limitations. The following table shows the support status for main Compose features.

:::info

If you rely on a specific Compose feature that is not supported by Uncloud, please submit a feature request in
[GitHub Discussions](https://github.com/psviderski/uncloud/discussions) or on [Discord](https://uncloud.run/discord).

:::

| Feature                          | Support Status     | Notes                                                                                                                                      |
|----------------------------------|--------------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| **Services**                     |                    |                                                                                                                                            |
| `build`                          | ✅ Supported        | Build context and Dockerfile                                                                                                               |
| `cap_add`                        | ✅ Supported        | Additional kernel [capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)                                                |
| `cap_drop`                       | ✅ Supported        | Which kernel [capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html) to drop                                             |
| `command`                        | ✅ Supported        | Override container command                                                                                                                 |
| `configs`                        | ✅ Supported        | File-based and inline configs                                                                                                              |
| `cpus`                           | ✅ Supported        | CPU limit                                                                                                                                  |
| `depends_on`                     | ⚠️ Limited         | Deployed in dependency order. Use [pre-deploy hooks](../4-guides/1-deployments/5-pre-deploy-hooks.md) for `service_completed_successfully` |
| `devices`                        | ✅ Supported        | Device mappings                                                                                                                            |
| `dns`                            | ❌ Not supported    | Built-in service discovery                                                                                                                 |
| `dns_search`                     | ❌ Not supported    | Built-in service discovery                                                                                                                 |
| `entrypoint`                     | ✅ Supported        | Override container entrypoint                                                                                                              |
| `env_file`                       | ✅ Supported        | Environment file                                                                                                                           |
| `environment`                    | ✅ Supported        | Environment variables                                                                                                                      |
| `gpus`                           | ✅ Supported        | GPU device access                                                                                                                          |
| `healthcheck`                    | ✅ Supported        | Health check configuration                                                                                                                 |
| `image`                          | ✅ Supported        | Container image specification                                                                                                              |
| `init`                           | ✅ Supported        | Run init process in container                                                                                                              |
| `labels`                         | ❌ Not supported    |                                                                                                                                            |
| `links`                          | ❌ Not supported    | Use service names for communication                                                                                                        |
| `logging`                        | ✅ Supported        | Defaults to [local](https://docs.docker.com/engine/logging/drivers/local/) log driver                                                      |
| `mem_limit`                      | ✅ Supported        | Memory limit                                                                                                                               |
| `mem_reservation`                | ✅ Supported        | Memory reservation                                                                                                                         |
| `mem_swappiness`                 | ❌ Not supported    |                                                                                                                                            |
| `memswap_limit`                  | ❌ Not supported    |                                                                                                                                            |
| `networks`                       | ❌ Not supported    | All containers share cluster network                                                                                                       |
| `pid`                            | ✅ Supported        | Set the PID namespace mode, `pid: host` only                                                                                               |
| `ports`                          | ⚠️ Limited         | `mode: host` only, use [`x-ports`](2-extensions.md#x-ports) for HTTP/HTTPS                                                                 |
| `privileged`                     | ✅ Supported        | Run containers in privileged mode                                                                                                          |
| `pull_policy`                    | ✅ Supported        | `always`, `missing`, `never`                                                                                                               |
| `secrets`                        | ❌ Not supported    | Use configs or environment variables                                                                                                       |
| `security_opt`                   | ❌ Not supported    |                                                                                                                                            |
| `shm_size`                       | ✅ Supported        | Shared memory size                                                                                                                         |
| `stop_grace_period`              | ✅ Supported        | Time to wait after SIGTERM before SIGKILL                                                                                                  |
| `storage_opt`                    | ❌ Not supported    |                                                                                                                                            |
| `sysctls`                        | ✅ Supported        | Namespaced kernel parameters                                                                                                               |
| `ulimits`                        | ✅ Supported        | Resource limits                                                                                                                            |
| `user`                           | ✅ Supported        | Set container user                                                                                                                         |
| `volumes`                        | ✅ Supported        | Named volumes, bind mounts, tmpfs                                                                                                          |
| **Deploy**                       |                    |                                                                                                                                            |
| `labels`                         | ❌ Not supported    |                                                                                                                                            |
| `mode`                           | ✅ Supported        | Either `global` or `replicated`                                                                                                            |
| `placement`                      | ❌ Not supported    | Use [`x-machines`](2-extensions.md#x-machines) extension                                                                                   |
| `replicas`                       | ✅ Supported        | Number of container replicas                                                                                                               |
| `resources`                      | ⚠️ Limited         | CPU, memory limits and device reservations                                                                                                 |
| `restart_policy`                 | ❌ Not supported    | Defaults to `unless-stopped`                                                                                                               |
| `rollback_config`                | ❌ Not supported    | See [#151](https://github.com/psviderski/uncloud/issues/151)                                                                               |
| `update_config`                  | ⚠️ Limited         | `order` and `monitor` supported. See [rolling deployments](../4-guides/1-deployments/4-rolling-deployments.md)                             |
| **Volumes**                      |                    |                                                                                                                                            |
| Named volumes                    | ✅ Supported        | Docker volumes                                                                                                                             |
| Bind mounts                      | ✅ Supported        | Host path binding                                                                                                                          |
| Tmpfs mounts                     | ✅ Supported        | In-memory filesystems                                                                                                                      |
| Volume labels                    | ✅ Supported        | Custom labels                                                                                                                              |
| External volumes                 | ✅ Supported        | Must exist before deployment                                                                                                               |
| [Volume drivers][volume-drivers] | ✅ Supported        | `local` (supports [NFS][volume-nfs], [CIFS/Samba][volume-cifs]) and manually installed third-party drivers                                 |
| **Configs**                      |                    |                                                                                                                                            |
| File-based configs               | ✅ Supported        | Read from file                                                                                                                             |
| Inline configs                   | ✅ Supported        | Defined in compose file                                                                                                                    |
| External configs                 | ❌ Not supported    | Not supported                                                                                                                              |
| Short syntax                     | ❌ Not supported    | Use long syntax only                                                                                                                       |
| **Extensions**                   |                    |                                                                                                                                            |
| `x-context`                      | ✅ Uncloud-specific | Cluster context override                                                                                                                   |
| `x-caddy`                        | ✅ Uncloud-specific | Custom Caddy configuration                                                                                                                 |
| `x-machines`                     | ✅ Uncloud-specific | Machine placement constraints                                                                                                              |
| `x-ports`                        | ✅ Uncloud-specific | Service port publishing                                                                                                                    |
| `x-pre_deploy`                   | ✅ Uncloud-specific | Pre-deploy hook command                                                                                                                    |

[volume-drivers]: https://docs.docker.com/engine/storage/volumes/#use-a-volume-driver

[volume-nfs]: https://docs.docker.com/engine/storage/volumes/#create-a-service-which-creates-an-nfs-volume

[volume-cifs]: https://docs.docker.com/engine/storage/volumes/#create-cifssamba-volumes

### Legend

- ✅ **Supported**: Feature works as documented
- ⚠️ **Limited**: Partial support or with restrictions
- ❌ **Not supported**: Feature is not (yet) available

See [Compose extensions](2-extensions.md) for details on additional Compose features provided by Uncloud.
