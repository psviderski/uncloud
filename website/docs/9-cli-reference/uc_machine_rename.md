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
  -h, --help   help for rename
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in the cluster.

