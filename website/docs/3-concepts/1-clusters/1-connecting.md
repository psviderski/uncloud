# Connecting to a cluster

`uc` only needs to reach one machine to work with the entire cluster. That machine acts as an **entry point** and
forwards requests to other machines as needed.

`uc` stores **cluster contexts** and **connection details** in a [configuration file](../../7-cli-config-reference.md)
(default location is `~/.config/uncloud/config.yaml`).

When you initialise a new cluster with `uc machine init` or add a machine to an existing cluster with `uc machine add`,
they automatically save the SSH addresses of your machines to the config so you don't have to specify them every time.

## Cluster contexts

The [config file](../../7-cli-config-reference.md) organises connections into **contexts**. Each context represents a
cluster. It has a name and a list of connection details for the machines in that cluster.

A context is not the same thing as a cluster. It is your local view of a cluster: which machines you can connect through
and in what order to try them. Different people or environments may need to reach the same cluster in different ways.

You can also manually create multiple contexts for the same cluster. For example, one that connects through
a machine with a public IP when you're not in the office, and another that connects through a private machine on the
office network when you're on-site to reduce latency. You can switch between them depending on where you are.

### Managing contexts

Use these commands to manage the contexts in your config:

- [`uc ctx`](../../9-cli-reference/uc_ctx.md): Switch contexts using an interactive TUI
- [`uc ctx ls`](../../9-cli-reference/uc_ctx_ls.md): List all contexts and see which one is current
- [`uc ctx use`](../../9-cli-reference/uc_ctx_use.md): Switch the current context by name
- [`uc ctx conn`](../../9-cli-reference/uc_ctx_connection.md): Change the default connection for the current context
  using an interactive TUI

You can also set `x-context` in your Compose file to pin a specific context for deployments. See
[Deploy to a specific cluster context](../../4-guides/1-deployments/1-deploy-app.md#deploy-to-a-specific-cluster-context)
for details.

## Connection resolution

When you run a `uc` command, it determines which cluster to connect to using this priority:

1. If `--connect` is set, `uc` connects directly to that machine and ignores the config file entirely.
2. If `--context` is set, `uc` uses that context from the config.
3. Otherwise, `uc` uses `current_context` from the config.

Once the context is resolved, `uc` tries each connection in the context's `connections` list in order until one
succeeds.

## Global flags and environment variables

These flags are available on every `uc` command. They can also be set with an environment variable. The flag takes
priority if both are set.

| Flag               | Environment variable | Description                                                       |
|--------------------|----------------------|-------------------------------------------------------------------|
| `--uncloud-config` | `UNCLOUD_CONFIG`     | Path to the config file                                           |
| `--context`        | `UNCLOUD_CONTEXT`    | Use a specific context instead of `current_context` in the config |
| `--connect`        | `UNCLOUD_CONNECT`    | Bypass the config file and connect directly                       |

### Connecting directly without a config

The `--connect` flag or `UNCLOUD_CONNECT` environment variable let you run one-off commands against a cluster without
using a config. This is useful for CI pipelines and scripts where you don't want to set up a config file.

It accepts these formats:

```shell
# System 'ssh' command with full SSH config support
uc --connect root@203.0.113.1 ls

# System 'ssh' command (explicit scheme, same as above)
uc --connect ssh://root@203.0.113.1 ls

# Go's built-in SSH library (no SSH config support, useful when the system ssh is not available)
uc --connect ssh+go://root@203.0.113.1 ls

# Direct connection to machine gRPC API over TCP (for advanced users with custom setups)
uc --connect tcp://[fdcc:4439:f545:3ca:5d17:66e5:7c96:40bd]:51000 ls

# Direct connection to machine gRPC API over a Unix socket (for running uc locally on a cluster machine)
uc --connect unix:///run/uncloud/uncloud.sock ls
```

:::info

Don't use `--connect` with `uc machine init`. `--connect` is for specifying or overriding the connection to an existing
cluster, but `uc machine init` creates a new one and writes the new cluster context to the config file. You can discard
the config when initialising a cluster with `--uncloud-config /dev/null` if you don't want to save it.

:::
