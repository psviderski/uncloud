# Deploy a global service

Deploy exactly one replica of a service on each machine in your cluster.

This is useful for cluster-wide infrastructure services like monitoring or security agents, log collectors, or reverse
proxies.

## Deploy to all machines

To deploy exactly one replica on each machine in your cluster for a specific service, set
[`mode: global`](https://github.com/compose-spec/compose-spec/blob/main/deploy.md#mode) under the `deploy` section in
your Compose file:

```yaml title="compose.yaml"
services:
  monitoring:
    image: quay.io/prometheus/node-exporter:latest
    deploy:
      # Run one container on each machine in the cluster
      mode: global
```

Then deploy:

```shell
uc deploy
```

Before creating replicas on cluster machines, it will show you a deployment plan and ask for confirmation.

If you add more machines to the cluster later, you need to run `uc deploy` again to create replicas on the new machines.
Uncloud doesn't automatically scale global services to new machines.

## Deploy to a subset of machines

You can combine the `global` mode with [`x-machines`](../../8-compose-file-reference/1-support-matrix.md#x-machines)
to deploy one container to each specified machine:

```yaml title="compose.yaml"
services:
  caddy:
    image: caddy:2
    deploy:
      # Run one container on each of the three specified machines
      mode: global
    x-machines:
      - ingress-1
      - ingress-2
      - ingress-3
```

This is useful when you want a service on a specific group of machines (for example, ingress or GPU machines) but still
want the one-per-machine guarantee that global mode provides.

## Global vs replicated mode

The default mode is `replicated`, where you specify the number of replicas.

| Mode                   | Replicas                                      | Placement                                                           |
|------------------------|-----------------------------------------------|---------------------------------------------------------------------|
| `replicated` (default) | You specify with `scale` or `deploy.replicas` | Uncloud evenly spreads replicas across all machines or `x-machines` |
| `global`               | Always one per machine                        | One replica on each machine or each `x-machines` machine            |

## See also

- [Deploy to specific machines](2-deploy-specific-machines.md): Deploy services to specific machines in your cluster
- [Compose Specification: deploy.mode](https://github.com/compose-spec/compose-spec/blob/main/deploy.md#mode):
  Compose specification for deployment modes
