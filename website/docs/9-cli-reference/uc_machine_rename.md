# uc machine rename

Rename a machine in the cluster.

## Synopsis

Rename a machine in the cluster.

This command changes the name of an existing machine while preserving all other
configuration including network settings, public IP, and cluster membership.

```
uc machine rename OLD_NAME NEW_NAME [flags]
```

## Options

```
  -c, --context string   Name of the cluster context. (default is the current context)
  -h, --help             help for rename
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file.
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in an Uncloud cluster.

