# uc service

Manage services in the cluster.

## Options

```
  -h, --help   help for service
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
* [uc service exec](uc_service_exec.md)	 - Execute a command in a running service container.
* [uc service inspect](uc_service_inspect.md)	 - Display detailed information on a service.
* [uc service logs](uc_service_logs.md)	 - View service logs.
* [uc service ls](uc_service_ls.md)	 - List services.
* [uc service rm](uc_service_rm.md)	 - Remove one or more services.
* [uc service run](uc_service_run.md)	 - Run a service.
* [uc service scale](uc_service_scale.md)	 - Scale a replicated service by changing the number of replicas.
* [uc service start](uc_service_start.md)	 - Start one or more services.
* [uc service stop](uc_service_stop.md)	 - Stop one or more services.

