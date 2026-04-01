`# CLI configuration file

The [`uc`](./2-getting-started/1-install-cli.md) CLI stores **cluster connection details** in a YAML configuration file.
Every time you run a command like `uc ls` or `uc deploy`, it reads this file to figure out which cluster to connect to
and how to reach its machines over SSH.

You rarely need to edit this file by hand. `uc` creates and updates it automatically when you run commands like
`uc machine init`, `uc machine add`, or `uc ctx`.

## Config location

The default config file path is `~/.config/uncloud/config.yaml`. You can change this with the `--uncloud-config`
global flag or the `UNCLOUD_CONFIG` environment variable:

```shell
# Use a custom config file for this command
uc --uncloud-config ./my-config.yaml ls

# Or set the environment variable to use a custom config file for all commands
export UNCLOUD_CONFIG=~/my-uncloud-config.yaml
uc ls
```

## Config structure

Here is an example config file for a setup with two clusters, `prod` and `dev`:

```yaml title="~/.config/uncloud/config.yaml"
current_context: prod
contexts:
  prod:
    connections:
      - ssh: user@137.123.45.67
      - ssh: myserver  # host alias from SSH config
  dev:
    connections:
      - ssh: ubuntu@dev.example.com
        ssh_key_file: ~/.ssh/id_ed25519
        machine_id: 3f60c88ac6b0f250d8aecb92a090922f
```

The config has two top-level attributes: `current_context` and `contexts`.

### `current_context`

The name of the active context. `uc` uses this context by default when you run any command. You can switch to a
different context with an interactive [`uc ctx`](9-cli-reference/uc_ctx.md) command, non-interactively with
[`uc ctx use`](9-cli-reference/uc_ctx_use.md), or the `--context` flag.

### `contexts`

A map of named contexts. Each context represents a cluster and contains a list of machine connections.

A context is not the same thing as a cluster. It is your local view of a cluster: which machines you can connect through
and in what order to try them. Two people on the same team can have different contexts pointing to the same cluster,
each with their own preferred entry point.

You can also create multiple contexts for the same cluster. For example, one that connects through a machine with a
public IP when you're not in the office, and another that connects through a private machine on the office network when
you're on-site to reduce latency.

### `connections`

Each connection in the list represents a machine in the cluster. `uc` only needs to reach one machine to work with the
entire cluster. That machine acts as an **entry point** and forwards requests to other machines as needed.

The first connection in the list is the default one that `uc` tries first. If it's unavailable, `uc` moves on to the
next one. You can change the default connection with an interactive command
[`uc ctx conn`](9-cli-reference/uc_ctx_connection.md).

Every connection must have exactly one connection type attribute:

| Attribute | Format                      | Description                                                                                                                        |
|-----------|-----------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `ssh`     | `user@host[:port]`          | Connect using the system `ssh` command with full SSH config support (default for new connections added with `uc machine init/add`) |
| `ssh_go`  | `user@host[:port]`          | Connect using the Go's built-in SSH library (no SSH config support)                                                                |
| `tcp`     | `host:port`                 | Connect directly to the machine gRPC API over TCP (for advanced users with custom setups)                                          |
| `unix`    | `/run/uncloud/uncloud.sock` | Connect directly to the machine gRPC API over a Unix socket (for running `uc` locally on the cluster machines)                     |

A connection can also have these optional attributes:

| Attribute      | Description                                                                                                             |
|----------------|-------------------------------------------------------------------------------------------------------------------------|
| `ssh_key_file` | Path to the SSH private key for this machine (maps to `-i` in the `ssh` command)                                        |
| `machine_id`   | Unique identifier of the machine (used to match and remove the connection when removing a machine with `uc machine rm`) |

## How the config gets created

You don't need to create this file yourself. `uc` manages it for you:

1. **`uc machine init`** creates a new context and adds the first machine connection. If you don't specify a context
   name with `-c`, it uses `default`. If `default` already exists, it auto-increments to `default-1`, `default-2`, and
   so on.
2. **`uc machine add`** appends a new machine connection to the current context.

For example, after initialising a cluster:

```shell
uc machine init root@203.0.113.1 -c prod
```

`uc` creates a config file that looks like this:

```yaml title="~/.config/uncloud/config.yaml"
current_context: prod
contexts:
  prod:
    connections:
      - ssh: root@203.0.113.1
        ssh_key_file: ~/.ssh/id_ed25519
        machine_id: a1b2c3d4e5f6
```

## Connection resolution

When you run a command, `uc` determines which cluster to connect to using this priority:

1. If `--connect` is set, `uc` connects directly to that machine and ignores the config file entirely.
2. If `--context` is set, `uc` uses that context from the config.
3. Otherwise, `uc` uses `current_context` from the config.

Once the context is resolved, `uc` tries each connection in the context's list in order until one succeeds.

## Global flags and environment variables

These flags are available on every `uc` command. Each flag can also be set with an environment variable. The flag takes
priority if both are set.

| Flag               | Environment variable | Description                                       |
|--------------------|----------------------|---------------------------------------------------|
| `--uncloud-config` | `UNCLOUD_CONFIG`     | Path to the config file                           |
| `--context`        | `UNCLOUD_CONTEXT`    | Use a specific context instead of the current one |
| `--connect`        | `UNCLOUD_CONNECT`    | Bypass the config file and connect directly       |

### The `--connect` flag

The `--connect` flag lets you run one-off commands against a machine without setting up a config file. It accepts these
formats:

```shell
# System 'ssh' command (default)
uc --connect root@203.0.113.1 ls

# System 'ssh' command (explicit scheme)
uc --connect ssh://root@203.0.113.1 ls

# Go's built-in SSH (no SSH config support)
uc --connect ssh+go://root@203.0.113.1 ls

# Direct TCP
uc --connect tcp://10.210.0.1:1234 ls

# Unix socket
uc --connect unix:///run/uncloud/uncloud.sock ls
```

:::warning

You can't use `--connect` with `uc machine init`. The `init` command needs the config file to save the new cluster
context.

:::

## Managing contexts

Use these commands to work with contexts in your config:

- [`uc ctx`](9-cli-reference/uc_ctx.md): Switch contexts using an interactive TUI
- [`uc ctx ls`](9-cli-reference/uc_ctx_ls.md): List all contexts and see which one is current
- [`uc ctx use`](9-cli-reference/uc_ctx_use.md): Switch the current context
- [`uc ctx conn`](9-cli-reference/uc_ctx_connection.md): Change the default connection for the current context

You can also set `x-context` in your Compose file to pin a specific context for deployments. See
[Deploy to a specific cluster context](4-guides/1-deployments/1-deploy-app.md#deploy-to-a-specific-cluster-context)
for details.
