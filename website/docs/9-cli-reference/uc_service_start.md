# uc service start

Start one or more services.

## Synopsis

Start one or more previously stopped services.

Starts all containers of the specified service(s) across all machines in the cluster.
Services can be specified by name or ID.

```
uc service start SERVICE [SERVICE...] [flags]
```

## Options

```
  -h, --help   help for start
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc service](uc_service.md)	 - Manage services in the cluster.

