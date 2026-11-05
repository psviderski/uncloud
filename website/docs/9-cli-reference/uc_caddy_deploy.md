# uc caddy deploy

Deploy or upgrade Caddy reverse proxy across all machines in the cluster.

## Synopsis

Deploy or upgrade Caddy reverse proxy across all machines in the cluster.
A rolling update is performed when updating existing containers to minimise disruption.

By default, this deploys the latest version of the Uncloud Caddy image with a global config that enables cluster
storage for TLS certificates. See https://github.com/unlabs-dev/caddy-uncloud for more details.

Values set with --image, --caddyfile, and --machine are kept for later deployments. Pass an empty value to reset
a flag to its default.

If this command didn't deploy the current caddy service, for example, it was deployed with 'uc deploy'
from a Compose file or an older version of uc, the deployment replaces its configuration.

Requires Uncloud daemon 0.22.0 or newer on all machines.

```
uc caddy deploy [flags]
```

## Examples

```
  # Deploy Caddy or upgrade it to the latest version of the Uncloud Caddy image.
  uc caddy deploy

  # Deploy a custom image and keep it for later deployments.
  uc caddy deploy --image ghcr.io/unlabs-dev/caddy-uncloud:0.1.3

  # Go back to the latest version of the Uncloud Caddy image.
  uc caddy deploy --image ""

  # Deploy a custom image build and a custom global Caddy config and keep both for later deployments.
  uc caddy deploy --image myorg/caddy-uncloud:latest --caddyfile global.Caddyfile

  # Go back to the default global Caddy config but keep the custom image.
  uc caddy deploy --caddyfile ""

  # Deploy only to specific machines and keep them for later deployments.
  uc caddy deploy --machine machine1,machine2

  # Go back to deploying to all machines.
  uc caddy deploy --machine ""
```

## Options

```
      --caddyfile string   Path to a custom global Caddy config (Caddyfile) that will be prepended to the auto-generated Caddy config.
                           An empty value resets it to the default config that enables cluster storage.
                           (default is the current custom config or the default config)
  -h, --help               help for deploy
      --image string       Caddy Docker image to deploy. It must include the Uncloud storage module if the Caddy config uses it.
                           An empty value resets it to the latest version of the Uncloud Caddy image.
                           (default is the current custom image or ghcr.io/unlabs-dev/caddy-uncloud:LATEST_VERSION)
  -m, --machine strings    Machine names or IDs to deploy to. Can be specified multiple times or as a comma-separated list.
                           An empty value resets it to all machines.
                           (default is the current machines or all machines)
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc caddy](uc_caddy.md)	 - Manage Caddy reverse proxy service.

