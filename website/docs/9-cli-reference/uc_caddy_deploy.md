# uc caddy deploy

Deploy or upgrade Caddy reverse proxy across all machines in the cluster.

## Synopsis

Deploy or upgrade Caddy reverse proxy across all machines in the cluster.
A rolling update is performed when updating existing containers to minimise disruption.

```
uc caddy deploy [flags]
```

## Options

```
      --caddyfile string   Path to a custom global Caddy config (Caddyfile) that will be prepended to the auto-generated Caddy config.
  -c, --context string     Name of the cluster context to deploy to. (default is the current context)
  -h, --help               help for deploy
      --image string       Caddy Docker image to deploy. (default caddy:LATEST_VERSION)
  -m, --machine strings    Machine names to deploy to. Can be specified multiple times or as a comma-separated list of machine names. (default is all machines)
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file.
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc caddy](uc_caddy.md)	 - Manage Caddy reverse proxy service.

