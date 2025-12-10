# uc volume inspect

Display detailed information on a volume.

```
uc volume inspect VOLUME_NAME [flags]
```

## Options

```
  -h, --help             help for inspect
  -m, --machine string   Name or ID of the machine where the volume is located. If not specified, the volume will be searched across all machines.
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

