# User Guide

## Initialising a cluster

To begin setting up a new Uncloud cluster, create the desired nodes. Ensure that their firewall allows the required ports.

### Ports

Nodes running Uncloud with a standard configuration must have the following inbound ports allowed in the firewall:

* 51820/udp (WireGuard meshnet)
* 22/tcp (default SSH management port)
* 80/tcp (required for challenge if uncloud.run DNS is enabled, pass --no-dns during init to disable)

In addition, any ports for any workloads you want to expose will need to be open.

### Configuration

Uncloud stores its configuration in `~/.config/uncloud/config.yaml`. If you wish to reinitialise a cluster, simply remove it from this config.

### Initialisation

Begin by initialising the first node in your cluster with `uc machine init [USER@HOST:PORT]`. If you do not have a need for Caddy reverse proxy, you may disable this feature with `--no-caddy`. If you want to avoid using uncloud's managed DNS service, add the `--no-dns` flag.

This command will idempotently install Docker, uncloudd, uncloud-corrosion. If Caddy is enabled, it will set up a reverse proxy. If Uncloud DNS is enabled, it will create a DNS A record for the machine's public IP address under `*.[CLUSTER ID].uncld.dev`.

If you wish to uninstall Uncloud and its components, run `uncloud-uninstall`.

### Adding a node

Just like the initialisation of the first node, a node can be added to the cluster with `uc machine add`.

### DNS

Uncloud (uncloud.run) DNS can be managed with the `uc dns` subcommand.

* To reserve a domain name, run `uc dns reserve`
* To release a domain name, run `uc dns release`.
* To see the domain name, run `uc dns show`
* To avoid using the Uncloud managed DNS service, use the `--no-dns` flag on your `uc machine init` command.

### Running a service

Services on an Uncloud cluster can be managed with `uc service`.

You can run a service with two replicas that expose port 80 like the following:

```
uc service run -p 80/http --replicas 2 nginxdemos/hello
```

This requires the Caddy reverse proxy to be deployed. If it wasn't previously, it can be deployed with `uc caddy deploy`.

Since this doesn't specify a service name, a random one will be generated.

The service can be deleted by its service name:

```
uc service rm hello-gsdo
```
