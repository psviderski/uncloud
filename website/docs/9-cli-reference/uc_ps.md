# uc ps

List all service containers.

## Synopsis

List all service containers across all machines in the cluster.

This command provides a comprehensive overview of all running containers that are part of a service,
making it easy to see the distribution and status of containers across the cluster.

```
uc ps [flags]
```

## Options

```
  -h, --help          help for ps
  -s, --sort string   Sort containers by 'service', 'machine', or 'health'. (default "service")
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

