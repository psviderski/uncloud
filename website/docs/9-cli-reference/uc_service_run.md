# uc service run

Run a service.

```
uc service run IMAGE [COMMAND...] [flags]
```

## Options

```
      --caddyfile string    Path to a custom Caddy config (Caddyfile) for the service. Cannot be used together with non-@host published ports.
      --cpu decimal         Maximum number of CPU cores a service container can use. Fractional values are allowed: 0.5 for half a core or 2.25 for two and a quarter cores.
      --entrypoint string   Overwrite the default ENTRYPOINT of the image. Pass an empty string "" to reset it.
  -e, --env strings         Set an environment variable for service containers. Can be specified multiple times.
                            Format: VAR=value or just VAR to use the value from the local environment.
  -h, --help                help for run
  -m, --machine strings     Placement constraint by machine names, limiting which machines the service can run on. Can be specified multiple times or as a comma-separated list of machine names. (default is any suitable machine)
      --memory bytes        Maximum amount of memory a service container can use. Value is a positive integer with optional unit suffix (b, k, m, g). Default unit is bytes if no suffix specified.
                            Examples: 1073741824, 1024m, 1g (all equal 1 gibibyte)
      --mode string         Replication mode of the service: either 'replicated' (a specified number of containers across the machines) or 'global' (one container on every machine). (default "replicated")
  -n, --name string         Assign a name to the service. A random name is generated if not specified.
      --privileged          Give extended privileges to service containers. This is a security risk and should be used with caution.
  -p, --publish strings     Publish a service port to make it accessible outside the cluster. Can be specified multiple times.
                            Format: [hostname:]container_port[/protocol] or [host_ip:]host_port:container_port[/protocol]@host
                            Supported protocols: tcp, udp, http, https (default is tcp). If a hostname for http(s) port is not specified
                            and a cluster domain is reserved, service-name.cluster-domain will be used as the hostname.
                            Examples:
                              -p 8080/https                  Publish port 8080 as HTTPS via reverse proxy with default service-name.cluster-domain hostname
                              -p app.example.com:8080/https  Publish port 8080 as HTTPS via reverse proxy with custom hostname
                              -p 53:5353/udp@host            Bind UDP port 5353 to host port 53
      --pull string         Pull image from the registry before running service containers ('always', 'missing', 'never'). (default "missing")
      --replicas uint       Number of containers to run for the service. Only valid for a replicated service. (default 1)
  -u, --user string         User name or UID and optionally group name or GID used for running the command inside service containers.
                            Format: USER[:GROUP] or UID[:GID]. If not specified, the user is set to the default user of the image.
  -v, --volume strings      Mount a data volume or host path into service containers. Service containers will be scheduled on the machine(s) where
                            the volume is located. Can be specified multiple times.
                            Format: volume_name:/container/path[:ro|volume-nocopy] or /host/path:/container/path[:ro]
                            Examples:
                              -v postgres-data:/var/lib/postgresql/data  Mount volume 'postgres-data' to /var/lib/postgresql/data in container
                              -v /data/uploads:/app/uploads         	 Bind mount /data/uploads host directory to /app/uploads in container
                              -v /host/path:/container/path:ro 		 Bind mount a host directory or file as read-only
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

