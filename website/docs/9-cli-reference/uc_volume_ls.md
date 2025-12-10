# uc volume ls

List volumes across all machines in the cluster.

```
uc volume ls [flags]
```

## Options

```
  -h, --help              help for ls
  -m, --machine strings   Filter volumes by machine name or ID. Can be specified multiple times or as a comma-separated list. (default is include all machines)
  -q, --quiet             Only display volume names.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc volume](uc_volume.md)	 - Manage volumes in the cluster.

