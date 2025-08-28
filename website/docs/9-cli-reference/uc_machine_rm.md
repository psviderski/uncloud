# uc machine rm

Remove a machine from a cluster and reset it.

```
uc machine rm MACHINE [flags]
```

## Options

```
  -c, --context string   Name of the cluster context. (default is the current context)
  -h, --help             help for rm
      --no-reset         Do not reset the machine after removing it from the cluster. This will leave all containers and data intact.
  -y, --yes              Do not prompt for confirmation before removing the machine.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file.
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in an Uncloud cluster.

