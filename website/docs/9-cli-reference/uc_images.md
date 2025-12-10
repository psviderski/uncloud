# uc images

List images on machines in the cluster.

## Synopsis

List images on machines in the cluster. By default, on all machines. Optionally filter by image name.

```
uc images [IMAGE] [flags]
```

## Examples

```
  # List all images on all machines.
  uc images

  # List images on specific machine.
  uc images -m machine1

  # List images on multiple machines.
  uc images -m machine1,machine2

  # List images filtered by name (with any tag) on all machines.
  uc images myapp

  # List images filtered by name pattern on specific machine.
  uc images "myapp:1.*" -m machine1
```

## Options

```
  -h, --help              help for images
  -m, --machine strings   Filter images by machine name or ID. Can be specified multiple times or as a comma-separated list. (default is include all machines)
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.

