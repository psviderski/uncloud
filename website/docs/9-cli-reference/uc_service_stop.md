# uc service stop

Stop one or more services.

## Synopsis

Stop one or more services.

```
uc service stop SERVICE [SERVICE...] [flags]
```

## Options

```
  -h, --help            help for stop
  -s, --signal string   Signal to send to the container
  -t, --timeout int     Seconds to wait before killing the container
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

