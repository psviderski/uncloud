# Rolling deployments

How `uc deploy` updates your services without downtime and automatically rolls back on failure.

Uncloud uses a rolling deployment to update your service by replacing its containers **one at a time**. Before moving on
to the next container, Uncloud waits for the new one to pass [health monitoring](#health-monitoring). If it fails to
become healthy, Uncloud stops the deployment and rolls back that container to the old one. This keeps your service
available throughout the update.

For a service with three replicas and the default `start-first` [update order](#update-order), the deployment looks like
this:

1. Start new container #1, wait until healthy
2. Stop and remove old container #1
3. Start new container #2, wait until healthy
4. Stop and remove old container #2
5. Start new container #3, wait until healthy
6. Stop and remove old container #3

At every step, at least three containers are serving traffic.

## Update order

The **update order** controls whether Uncloud starts the new container before or after stopping the old one.

| Order         | What happens                                                                | Best for                            |
|---------------|-----------------------------------------------------------------------------|-------------------------------------|
| `start-first` | Start new container, then stop old<br/>(running containers briefly overlap) | Stateless services (web apps, APIs) |
| `stop-first`  | Stop old container, then start new                                          | Stateful services (databases)       |

The default is `start-first` so there's **no downtime**. But it automatically switches to `stop-first` in two cases:

- **Host port conflicts**: the old container must free the port before the new one can bind to it.
- **Single-replica service with a volume**: two containers simultaneously writing to the same volume can
  **corrupt data**, so Uncloud stops the old container first to prevent this.

`stop-first` can cause a **brief downtime** while the old container stops and the new one starts in these cases. The
deployment plan printed by `uc deploy` indicates which containers will be replaced with `stop-first`.

A multiple-replica service with a volume doesn't automatically switch to `stop-first` as Uncloud assumes that the
concurrent access is desired and safe. Host path and tmpfs mounts don't trigger the switch either.

### Override update order

You can override the update order with `deploy.update_config.order`:

```yaml title="compose.yaml"
services:
  app:
    image: myapp
    volumes:
      - data:/data
    deploy:
      update_config:
        order: start-first

volumes:
  data:
```

This single-replica service uses a volume, so Uncloud would normally use `stop-first`. Setting `order: start-first`
overrides that.

This is useful if your app handles concurrent access to data safely and you want to avoid downtime. For example, the app
uses an SQLite database in WAL mode on the volume.

## Health monitoring

After starting each new container, Uncloud **monitors** it for failures for **5 seconds** to make sure it keeps running
and not crashing. If it keeps restarting after this period, the deployment fails and Uncloud
[rolls back](#rollback-on-failure) that container to the old one.

This is a safeguard to prevent you from deploying broken code or misconfiguration that would cause downtime. 5 seconds
is typically enough for a process in a container to initialise all its dependencies and start.

You can change the monitoring period for a service with `deploy.update_config.monitor`. For example, increase it if your
app takes longer to start or if you want to give it more time to recover from transient errors on startup.

```yaml title="compose.yaml"
services:
  app:
    image: myapp
    deploy:
      update_config:
        # Specified as duration: 500ms, 20s, 1m30s, 0s (skip)
        monitor: 10s
```

Set it to `0s` to skip monitoring entirely if you are confident the new containers will start correctly and want to
speed up the deployment. A safer alternative is to configure a [health check](#health-checks) instead.

You can also change the default monitoring period (`5s`) for all services globally with an environment variable
`UNCLOUD_HEALTH_MONITOR_PERIOD`:

```shell
export UNCLOUD_HEALTH_MONITOR_PERIOD=10s

# or skip monitoring for all services
export UNCLOUD_HEALTH_MONITOR_PERIOD=0s
```

`deploy.update_config.monitor` overrides the global default for that service.

### Health checks

If your service has a [`healthcheck`](https://github.com/compose-spec/compose-spec/blob/main/spec.md#healthcheck)
configured, Uncloud also checks its health status during and after the monitoring period.

If a container becomes `healthy` before the monitoring period ends, the deployment succeeds early and moves on to the
next container. If the container is `unhealthy` after the monitoring period, Uncloud
[rolls it back](#rollback-on-failure) and fails the deployment. Transient `unhealthy` states during the monitoring
period are tolerated to give the container time to recover from startup issues.

To make deployments **safer** and **faster**, it's recommended to configure a health check that can quickly notify
Uncloud when containers start successfully and become ready to serve traffic. You can configure it with
[`healthcheck`](https://github.com/compose-spec/compose-spec/blob/main/spec.md#healthcheck) in your Compose file or
[`HEALTHCHECK`](https://docs.docker.com/reference/dockerfile#healthcheck) in your image `Dockerfile`:

```yaml title="compose.yaml"
services:
  app:
    image: myapp
    healthcheck:
      test: curl -f http://localhost:8000/health
      interval: 5s
      retries: 3
      start_period: 10s
      start_interval: 1s
```

:::info important

If a health check fails after the deployment, Uncloud automatically removes the unhealthy container from the
[Caddy](../../3-concepts/2-ingress/1-overview.md) configuration to prevent routing traffic to that container. But it
doesn't automatically restart or roll it back.

Uncloud automatically adds it back to Caddy when it recovers and becomes healthy again. You can inspect the health
status of your containers with [`uc ps`](../../9-cli-reference/uc_ps.md) or
[`uc inspect`](../../9-cli-reference/uc_inspect.md) and check their logs with
[`uc logs`](../../9-cli-reference/uc_logs.md).

:::

### Skip health monitoring

To skip health monitoring for **faster emergency deployments**, use `uc deploy --skip-health`.

:::warning

`--skip-health` won't detect containers that crash on startup or become unhealthy so won't roll them back or stop the
deployment. Use this only for emergency deployments when you are confident the new containers will start correctly.

:::

## Rollback on failure

If a new container fails health monitoring during a deployment, Uncloud stops it but keeps it around for inspection. For
`stop-first` order, Uncloud also restarts the old container. The deployment then stops and the remaining containers are
left untouched.

For example, if the first container in a rolling update succeeds but the second one fails, the first replacement stays
in place.

### Failed container logs

To help you diagnose the failure, `uc deploy` prints the last 10 log lines from the failed container.

You can change how many lines are printed with the `UNCLOUD_FAILED_CONTAINER_LOGS_TAIL` environment variable. Set it to
a number or to `all` to print the full container log:

```shell
export UNCLOUD_FAILED_CONTAINER_LOGS_TAIL=50
```

You can fetch the full logs with [`uc logs`](../../9-cli-reference/uc_logs.md) or inspect the status of the stopped
container with [`uc inspect`](../../9-cli-reference/uc_inspect.md) or [`uc ps`](../../9-cli-reference/uc_ps.md).

## Retry after failure

You can retry the deployment by running `uc deploy` again. Uncloud will skip the successfully deployed containers if the
configuration hasn't changed and only redeploy the remaining ones.

## See also

- [Pre-deploy hooks](5-pre-deploy-hooks.md): Run a command before deploying service containers
- [Deploy an app](1-deploy-app.md): Build and deploy from source code or pre-built images
- [Compose support matrix](../../8-compose-file-reference/1-support-matrix.md): Supported Compose features and Uncloud
  extensions
