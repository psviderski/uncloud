# uc

A CLI tool for managing Uncloud resources such as machines, services, and volumes.

## Options

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], or tcp://host:port
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
  -h, --help                    help for uc
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc build](uc_build.md)	 - Build services from a Compose file.
* [uc caddy](uc_caddy.md)	 - Manage Caddy reverse proxy service.
* [uc ctx](uc_ctx.md)	 - Switch between different cluster contexts. Contains subcommands to manage contexts.
* [uc deploy](uc_deploy.md)	 - Deploy services from a Compose file.
* [uc dns](uc_dns.md)	 - Manage cluster domain in Uncloud DNS.
* [uc exec](uc_exec.md)	 - Execute a command in a running service container
* [uc image](uc_image.md)	 - Manage images on machines in the cluster.
* [uc images](uc_images.md)	 - List images on machines in the cluster.
* [uc inspect](uc_inspect.md)	 - Display detailed information on a service.
* [uc ls](uc_ls.md)	 - List services.
* [uc machine](uc_machine.md)	 - Manage machines in an Uncloud cluster.
* [uc rm](uc_rm.md)	 - Remove one or more services.
* [uc run](uc_run.md)	 - Run a service.
* [uc scale](uc_scale.md)	 - Scale a replicated service by changing the number of replicas.
* [uc service](uc_service.md)	 - Manage services in an Uncloud cluster.
* [uc volume](uc_volume.md)	 - Manage volumes in an Uncloud cluster.

