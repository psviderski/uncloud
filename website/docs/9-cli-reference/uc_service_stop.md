# uc service stop

Stop one or more services.

## Synopsis

Stop one or more running services.

Gracefully stops all containers of the specified service(s) across all machines in the cluster.
Services can be specified by name or ID. Stopped services can be restarted with 'uc start'.

```
uc service stop SERVICE [SERVICE...] [flags]
```

## Options

```
  -h, --help            help for stop
  -s, --signal string   Signal to send to each container's main process.
                        Can be a signal name (SIGTERM, SIGINT, SIGHUP, etc.) or a number. (default SIGTERM)
  -t, --timeout int     Seconds to wait for each container to stop gracefully before forcibly killing it with SIGKILL.
                        Use -1 to wait indefinitely. (default 10)
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

