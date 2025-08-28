# uc volume ls

List volumes across all machines in the cluster.

```
uc volume ls [flags]
```

## Options

```
  -c, --context string    Name of the cluster context. (default is the current context)
  -h, --help              help for ls
  -m, --machine strings   Filter volumes by machine name or ID. Can be specified multiple times or as a comma-separated list. (default is include all machines)
  -q, --quiet             Only display volume names.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file.
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc volume](uc_volume.md)	 - Manage volumes in an Uncloud cluster.

