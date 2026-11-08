# uc volume export

Export a volume as a tar archive to standard output.

## Synopsis

Export a volume as a (compressed) tar archive to standard output.

The tar archive is created using GNU tar running in a container and outputs a gzipped archive to standard output.
A file can be created by redirecting the output to a file.

	uc volume export VOLUME_NAME > volume.tar.gz


```
uc volume export VOLUME_NAME [flags]
```

## Options

```
  -h, --help             help for export
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

