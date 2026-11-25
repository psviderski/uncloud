# uc volume import

Import a volume from a tar archive from standard input.

## Synopsis

Import a volume as (compressed) tar archive from standard input.

The tar archive is copied from standard input to a GNU tar running in a container.

If you have a (gzipped) tar archive you can import this to a new volume with:

	uc volume import VOLUME_NAME < volume.tar.gz

When importing the files are printed to standard output. Copying a volume on the fly can be done
by piping the output from 'uc volume export' into import:

	uc volume export OLD_VOLUME | uc volume import NEW_VOLUME


```
uc volume import VOLUME_NAME [flags]
```

## Options

```
  -h, --help             help for import
  -m, --machine string   Name or ID of the machine where the volume is located. If not specified, the volume will be searched across all machines.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc volume](uc_volume.md)	 - Manage volumes in the cluster.

