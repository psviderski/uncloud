# uc ctx create

Create the cluster context to Uncloud configuration file by connecting to the remote machine.

## Synopsis

Create the cluster context, or add new machines to an existing cluster context.
This command adds or updates an (existing) context in your Uncloud config with machines that have a public IP address
configured. By default the context is printed to standard output, use -w to write it to the Uncloud config.

Connection methods:
  [ssh://]user@host   - Use system 'ssh' command with full SSH config support (default, no prefix required)
  ssh+go://user@host  - Use Go's built-in SSH library

```
uc ctx create [schema://]USER@HOST[:PORT] [flags]
```

## Examples

```
  # Get the cluster context with default settings.
  uc context create -w root@<your-server-ip>

  # Add a new context named 'prod' in the Uncloud config (~/.config/uncloud/config.yaml).
  uc context create -w root@<your-server-ip> -c prod

  # Add a new context with a non-root user and custom SSH port and key.
  uc context create -w ubuntu@<your-server-ip>:2222 -i ~/.ssh/mykey
```

## Options

```
  -c, --context string   Name of the new context to be created in the Uncloud config to manage the cluster. (default "default")
  -h, --help             help for create
  -i, --ssh-key string   Path to SSH private key for remote login (if not already added to SSH agent). (default "~/.ssh/id_ed25519")
  -w, --write            Write a new Uncloud config, by default the config is only printed to standard output.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc ctx](uc_ctx.md)	 - Switch between different cluster contexts. Contains subcommands to manage contexts.

