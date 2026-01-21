# uc volume rm

Remove one or more volumes.

## Synopsis

Remove one or more volumes. You cannot remove a volume that is in use by a container.

```
uc volume rm VOLUME_NAME [VOLUME_NAME...] [flags]
```

## Options

```
  -f, --force             Force the removal of one or more volumes.
  -h, --help              help for rm
  -m, --machine strings   Name or ID of the machine to remove one or more volumes from. Can be specified multiple times or as a comma-separated list.
                          If not specified, the found volume(s) will be removed from all machines.
  -y, --yes               Do not prompt for confirmation before removing the volume(s).
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

