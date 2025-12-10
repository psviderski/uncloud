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
  -h, --help               help for deploy
      --image string       Caddy Docker image to deploy. (default caddy:LATEST_VERSION)
  -m, --machine strings    Machine names or IDs to deploy to. Can be specified multiple times or as a comma-separated list. (default is all machines)
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc caddy](uc_caddy.md)	 - Manage Caddy reverse proxy service.

