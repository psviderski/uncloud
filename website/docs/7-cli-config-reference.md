# CLI configuration file

The [`uc`](2-getting-started/1-install-cli.md) CLI stores **cluster connection details** in a YAML configuration file.
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
each with their own preferred connection.

See [Connecting to a cluster](3-concepts/1-clusters/1-connecting.md#cluster-contexts) for more details and examples.

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
| `ssh_go`  | `user@host[:port]`          | Connect using Go's built-in SSH library (no SSH config support)                                                                |
| `tcp`     | `host:port`                 | Connect directly to the machine gRPC API over TCP (for advanced users with custom setups)                                          |
| `unix`    | `/run/uncloud/uncloud.sock` | Connect directly to the machine gRPC API over a Unix socket (for running `uc` locally on the cluster machines)                     |

A connection can also have these optional attributes:

| Attribute      | Description                                                                                                             |
|----------------|-------------------------------------------------------------------------------------------------------------------------|
| `ssh_key_file` | Path to the SSH private key for this machine (maps to `-i` in the `ssh` command)                                        |
| `machine_id`   | Unique identifier of the machine (used to match and remove the connection when removing a machine with `uc machine rm`) |

## How the config gets created and updated

You don't typically need to create or edit the config file manually. `uc` creates and manages it for you:

- [`uc machine init`](9-cli-reference/uc_machine_init.md) creates a file if it doesn't exist and adds a new context with
  the first machine connection. If you don't specify a context name with `--context`, it uses `default`. If
  `default` already exists, it auto-increments to `default-1`, `default-2`, and so on. You can rename contexts by
  editing the config file.
- [`uc machine add`](9-cli-reference/uc_machine_add.md) appends a new machine connection to the current context. You can
  manually edit the connections in the config if needed.
- [`uc ctx`](9-cli-reference/uc_ctx.md) or [`uc ctx use`](9-cli-reference/uc_ctx_use.md) updates `current_context` when
  you switch contexts.
- [`uc ctx conn`](9-cli-reference/uc_ctx_connection.md) moves the selected connection to the top of the list to make it
  the default for that context.

For example, after initialising a cluster:

```shell
uc machine init root@203.0.113.1 --context prod
```

It creates a config file that looks like this:

```yaml title="~/.config/uncloud/config.yaml"
current_context: prod
contexts:
  prod:
    connections:
      - ssh: root@203.0.113.1
        machine_id: 4e81d368edf48480e5a2efb0915d6e38
```
