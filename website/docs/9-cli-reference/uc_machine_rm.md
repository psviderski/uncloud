# uc machine rm

Remove a machine from a cluster and reset it.

```
uc machine rm MACHINE [flags]
```

## Options

```
  -h, --help       help for rm
      --no-reset   Do not reset the machine after removing it from the cluster. This will leave all containers and data intact.
  -y, --yes        Do not prompt for confirmation before removing the machine.
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

