# uc volume inspect

Display detailed information on a volume.

```
uc volume inspect VOLUME_NAME [flags]
```

## Options

```
  -c, --context string   Name of the cluster context. (default is the current context)
  -h, --help             help for inspect
  -m, --machine string   Name or ID of the machine where the volume is located. If not specified, the volume will be searched across all machines.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file.
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc volume](uc_volume.md)	 - Manage volumes in an Uncloud cluster.

