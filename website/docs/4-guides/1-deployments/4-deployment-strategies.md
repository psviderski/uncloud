# Deployment strategies

When you run `uc deploy`, Uncloud updates your services without taking them offline. This page explains how deployments
work and how to configure them for different types of services.

## Rolling deployments

Uncloud uses rolling deployments: it replaces containers one at a time, waiting for each new container to start before
removing the old one. This keeps your service available throughout the update.

For a service with three replicas, the deployment looks like this:

1. Start new container #1
2. Remove old container #1
3. Start new container #2
4. Remove old container #2
5. Start new container #3
6. Remove old container #3

At every step, at least two containers are serving traffic.

:::note

Rolling is currently the only supported deployment strategy.

:::

## Update order

The **update order** controls whether Uncloud starts the new container before or after stopping the old one.

| Order | What happens | Best for |
|-------|--------------|----------|
| `start-first` | Start new container, then stop old | Stateless services (web apps, APIs) |
| `stop-first` | Stop old container, then start new | Stateful services (databases) |

### Default behavior

Uncloud picks the safest default based on your service:

- **Services with host port conflicts** use `stop-first` because ports must be freed first
- **Services with named volumes** (not bind mounts or tmpfs):
  - **Single replica** uses `stop-first` to prevent data corruption
  - **Multiple replicas** uses `start-first` since concurrent access is already happening
- **All other services** use `start-first` for zero downtime

### Overriding the default

Set `deploy.update_config.order` to override:

```yaml title="compose.yaml"
services:
  app:
    image: myapp
    deploy:
      update_config:
        order: start-first
    volumes:
      - app-data:/data

volumes:
  app-data:
```

This single-replica service has a volume, so Uncloud would normally use `stop-first`. Setting `order: start-first`
overrides that—useful if your app handles concurrent access safely (like SQLite in WAL mode).

### Choosing the right order

**Use `start-first`** when your service can run multiple instances simultaneously:

- Web applications and API servers
- Background workers processing independent jobs
- Read-heavy services with shared caches

**Use `stop-first`** when your service needs exclusive access:

- Databases (PostgreSQL, MySQL, Redis)
- Services with file locks
- Anything that writes to a volume without coordination

:::warning

Two containers writing to the same volume can corrupt your data. Uncloud defaults to `stop-first` for single-replica
services with volumes, but if you override this or use multiple replicas, make sure your application handles concurrent
access correctly.

:::

## See also

- [Deploy an app](1-deploy-app.md): Build and deploy from source or pre-built images
- [Compose support matrix](../../8-compose-file-reference/1-support-matrix.md): Supported Compose features
