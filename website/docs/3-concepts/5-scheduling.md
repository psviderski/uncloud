# Scheduling

When you deploy services, Uncloud decides which machines run which containers. This page explains how placement works and what you can control.

## Basic placement

Uncloud places containers one at a time, picking the best machine from those that qualify. For example, if you deploy three replicas across two machines:

```yaml title="compose.yaml"
services:
  api:
    image: myapp:latest
    deploy:
      replicas: 3
```

Uncloud spreads them across both machines — two on one, one on the other — rather than piling all three on a single machine.

## Constraining placement

You can control where containers run using three mechanisms.

### Machine restrictions

Use [`x-machines`](../8-compose-file-reference/1-support-matrix.md#x-machines) to limit which machines can run a service:

```yaml title="compose.yaml"
services:
  api:
    image: myapp:latest
    x-machines:
      - prod-server-1
      - prod-server-2
```

Only the listed machines are eligible. This is useful when workloads need specific hardware or locations.

### Volume requirements

If your service mounts a named volume, it can only run where that volume exists:

```yaml title="compose.yaml"
services:
  db:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

Uncloud handles volume placement automatically. If the volume doesn't exist yet, Uncloud creates it on an eligible machine before starting the container.

### Resource reservations

When you set CPU or memory reservations, machines must have enough available capacity:

```yaml title="compose.yaml"
services:
  api:
    image: myapp:latest
    deploy:
      replicas: 3
      resources:
        reservations:
          cpus: '0.5'
          memory: 512M
```

Each replica needs 0.5 CPU cores and 512MB reserved. If a machine runs out of capacity mid-deployment, remaining replicas go to other eligible machines.

:::tip
Reservations are opt-in. Without them, Uncloud round-robins across eligible machines without checking capacity.
:::

These constraints are passed to Docker, which enforces them at the container level. To set hard caps that containers can't exceed, use `limits` alongside reservations — see the [Compose support matrix](../8-compose-file-reference/1-support-matrix.md) for details.

## Volumes

Named volumes add an important constraint: containers must run where their volumes are.

### Shared volumes

When multiple services share a volume, they run on the same machine:

```yaml title="compose.yaml"
services:
  app:
    image: myapp:latest
    volumes:
      - uploads:/data/uploads
    deploy:
      resources:
        reservations:
          cpus: '1'

  worker:
    image: myapp:worker
    volumes:
      - uploads:/data/uploads
    deploy:
      resources:
        reservations:
          cpus: '1'

volumes:
  uploads:
```

Both `app` and `worker` share the `uploads` volume, so they'll be placed together. Uncloud checks upfront that a machine can fit their **combined** resource needs (2 CPU cores in this example).

### Volume spreading

When you have multiple independent volumes, Uncloud spreads them across machines rather than putting everything on one:

```yaml title="compose.yaml"
services:
  db1:
    volumes: [data1:/var/lib/data]
  db2:
    volumes: [data2:/var/lib/data]

volumes:
  data1:
  data2:
```

Here `data1` and `data2` are independent, so Uncloud places them on different machines when possible.

:::note
Uncloud schedules larger workloads first. Services needing more CPU or memory get placed before smaller ones, avoiding situations where small workloads take spots that larger ones needed.
:::

## Service modes

Uncloud supports two deployment modes:

**Replicated** (default): Runs the specified number of replicas, spread across eligible machines.

```yaml title="compose.yaml"
services:
  api:
    deploy:
      mode: replicated
      replicas: 3
```

**Global**: Runs exactly one container on every eligible machine. If any machine doesn't qualify, the deployment fails.

```yaml title="compose.yaml"
services:
  monitoring:
    deploy:
      mode: global
```

See [Deploy a global service](../4-guides/1-deployments/3-deploy-global-services.md) for more details.

## Troubleshooting

When Uncloud can't place a container, it tells you why. Here are common errors and what to do:

### No eligible machines

```
cannot schedule replica 1 of service 'api':
  node1: machine 'node1' not in allowed list: [prod-server-1, prod-server-2]
  node2: volume 'pgdata' not found on machine
```

**Causes:**
- `x-machines` excludes all available machines
- Required volumes don't exist on any eligible machine
- Resource reservations exceed available capacity on all machines

**Solutions:**
- Check your `x-machines` list includes the right machines
- Verify volumes exist where you expect ([`uc volume ls`](../9-cli-reference/uc_volume_ls.md))
- Review machine capacity with [`uc machine ls`](../9-cli-reference/uc_machine_ls.md)

### Insufficient resources for shared volume

```
insufficient resources for services 'app', 'worker' sharing volume 'uploads':
need 4.00 CPU cores and 2.00 GB memory combined
```

Services sharing a volume need more combined resources than any single machine has.

**Solutions:**
- Reduce CPU/memory reservations
- Add a machine with more capacity
- Split services to use separate volumes (if they don't actually need to share data)

### Uneven spread

If containers cluster on fewer machines than expected, some machines may be ineligible due to constraints or capacity. Use [`uc inspect`](../9-cli-reference/uc_inspect.md) to see where containers landed and check constraint satisfaction.

## See also

- [Deploy to specific machines](../4-guides/1-deployments/2-deploy-specific-machines.md) — using `x-machines`
- [Deploy a global service](../4-guides/1-deployments/3-deploy-global-services.md) — one replica per machine
- [Compose support matrix](../8-compose-file-reference/1-support-matrix.md) — all supported Compose features
