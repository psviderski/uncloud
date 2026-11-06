# Managing Caddy

Uncloud automatically deploys Caddy as a global service named `caddy` when you initialise a cluster with
`uc machine init`. By default, it runs on every machine and routes incoming HTTP/HTTPS traffic to your services.

Since [v0.22.0](https://github.com/psviderski/uncloud/releases/tag/v0.22.0), the default Caddy deployment uses the
Uncloud Caddy image
[`ghcr.io/unlabs-dev/caddy-uncloud`](https://github.com/unlabs-dev/caddy-uncloud/pkgs/container/caddy-uncloud) with
[cluster storage](4-cluster-storage-for-caddy.md) to share TLS certificates across machines. This image is a Caddy build
that includes the [Uncloud storage module](https://github.com/unlabs-dev/caddy-uncloud) and a few other popular modules
such as [caddy-l4](https://github.com/mholt/caddy-l4) and [cloudflare](https://github.com/caddy-dns/cloudflare).

:::warning

If you want to upgrade or redeploy Caddy on an existing cluster created with Uncloud v0.21.0 or earlier, follow
[Upgrading from earlier versions](4-cluster-storage-for-caddy.md#upgrading-from-earlier-versions) before redeploying
Caddy.

:::

## Checking status

View the `caddy` service status and which machines it's running on:

```shell
uc inspect caddy
```

```
Service ID: b5b269d5dc5ed4fdae6542894f94de82
Name:       caddy
Mode:       global

CONTAINER ID   IMAGE                                    CREATED        STATUS       IP ADDRESS   MACHINE
fb8f390e634d   ghcr.io/unlabs-dev/caddy-uncloud:0.1.4   3 weeks ago    Up 3 weeks   10.210.1.2   prod-ap1
0182f5d7bd9f   ghcr.io/unlabs-dev/caddy-uncloud:0.1.4   3 weeks ago    Up 3 weeks   10.210.0.2   prod-us1
```

## Verifying config

View the complete generated Caddyfile that the `caddy` service uses. It includes global configuration, sites for
published service ports, custom service configs, and comments explaining any skipped invalid configs. It also warns if
the last config load failed. Use `--machine NAME` to check a specific machine.

```shell
uc caddy config
```

See [Verifying Caddy config](2-publishing-services.md#verifying-caddy-config) for an example output and details on how
it's generated.

## Listing certificates

List the issued certificates in cluster storage:

```shell
uc caddy cert ls
```

```
ID             NAME                            ISSUER                    EXPIRES
c111e23615f7   dns.uncloud.run                 Let's Encrypt             2026-12-31 (2 months)
b5dcd2a03e1b   nginx.2t5ex2.uncld.dev          Let's Encrypt             2026-12-31 (2 months)
70707fd2fea2   test-staging.2t5ex2.uncld.dev   Let's Encrypt (staging)   2026-12-31 (2 months)
c5f9301e1c06   uncloud.run                     Let's Encrypt             2026-12-31 (2 months)
```

## Deploying or updating Caddy

Deploy Caddy or upgrade it to the latest version of the Uncloud Caddy image:

```shell
uc caddy deploy
```

The command shows a deployment plan and asks for confirmation, then performs a rolling update.

You can set a specific image, global Caddyfile, or limit deployment to specific machines:

```shell
# Deploy a specific version or custom image and keep it for later deployments.
uc caddy deploy --image ghcr.io/unlabs-dev/caddy-uncloud:0.1.4

# Deploy with a custom global Caddy config and only on selected machines.
uc caddy deploy --caddyfile global.Caddyfile --machine machine1,machine2
```

The command remembers these values for later deployments. Pass an empty value to reset a flag to its default:

```shell
uc caddy deploy --image "" --caddyfile "" --machine ""
```

See [`uc caddy deploy`](../../9-cli-reference/uc_caddy_deploy.md) for all flags and examples.

## Custom global configuration

A custom Caddyfile replaces the default global config. Uncloud combines this config with the sites generated from
[published ports](2-publishing-services.md) and
[custom service configs](2-publishing-services.md#custom-caddy-configuration). Include `storage uncloud` to keep using
[cluster storage](4-cluster-storage-for-caddy.md):

```caddyfile title="global.Caddyfile"
# Global options.
{
    storage uncloud
    # Uncomment for troubleshooting:
    # debug
}

# A snippet that can be reused in custom Caddy configs for services (x-caddy).
(my_snippet) {
    # ...
}

# Expose an internal service that is not managed by Uncloud.
internal.example.com {
    reverse_proxy 192.168.1.100
}
```

Deploy your custom global Caddy config:

```shell
uc caddy deploy --caddyfile global.Caddyfile
```

## Custom image

A custom image must include the [Uncloud storage module](https://github.com/unlabs-dev/caddy-uncloud) to use cluster
storage. It must also include `curl` for the health check and start Caddy with `/etc/caddy/Caddyfile`.

The default Uncloud Caddy image
[`ghcr.io/unlabs-dev/caddy-uncloud`](https://github.com/unlabs-dev/caddy-uncloud/pkgs/container/caddy-uncloud) satisfies
these requirements. See the [release notest](https://github.com/unlabs-dev/caddy-uncloud/releases/latest) for the Caddy
version and the list of included modules.

If you need other Caddy modules or want to choose a compatible Caddy version, you can
[build and deploy your own image](https://github.com/unlabs-dev/caddy-uncloud#building-and-deploying-a-custom-image).

## Managing Caddy with Compose

Use Compose when you need more control, such as environment variables for a DNS provider. This example uses the
Cloudflare module included in the default image to obtain a wildcard certificate for `*.example.com`:

<Tabs>
<TabItem value="compose.yaml">

```yaml
services:
  caddy:
    image: ghcr.io/unlabs-dev/caddy-uncloud:0.1.4
    environment:
      # unix// is not a typo. Caddy uses network/address format, not a unix:// URL.
      CADDY_ADMIN: unix//run/caddy/admin.sock
      CLOUDFLARE_API_TOKEN: secret://cloudflare_api_token
    healthcheck:
      test: curl -fsS -o /dev/null --unix-socket /run/caddy/admin.sock http://localhost/config/
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
      start_interval: 1s
    volumes:
      - /var/lib/uncloud/caddy:/etc/caddy:ro
      # Persists TLS certificates and other assets when using Caddy's default local storage.
      - /var/lib/uncloud/caddy:/data
      # Uncloud API socket is required by the cluster storage module (https://github.com/unlabs-dev/caddy-uncloud).
      # Mount the directory, not the socket file, so Caddy sees a replacement socket after the daemon restarts.
      - /run/uncloud/api:/run/uncloud/api:ro
      - /run/uncloud/caddy:/run/caddy
    x-ports:
      - 80:80@host
      - 443:443@host
      - 443:443/udp@host
    x-caddy: ./Caddyfile
    deploy:
      mode: global
    # Optional: deploy only to specific machines.
    # x-machines:
    #   - machine1
    #   - machine2

secrets:
  cloudflare_api_token:
    # Read the secret value from 1Password by running this command.
    x-command: op read "op://prod/caddy/cloudflare_api_token"
```

</TabItem>
<TabItem value="Caddyfile">

```caddyfile
# Global options.
{
    storage uncloud
}

# Obtain a wildcard TLS certificate for all subdomains of example.com using DNS challenge with Cloudflare.
# It will be used for services that publish ports with hostnames under example.com.
*.example.com {
    tls {
        dns cloudflare {env.CLOUDFLARE_API_TOKEN}
    }
    respond "No host matched" 404
}
```

</TabItem>
</Tabs>

Deploy or update the `caddy` service from the Compose file:

```shell
uc deploy
```

:::info note

Keep the `environment`, `volumes`, and `x-ports` properties as shown, including the source paths of the volume mounts.
The Uncloud daemon and the storage module rely on them.

:::

Continue using `uc deploy` for a Compose-managed Caddy service. Switching to `uc caddy deploy` replaces its
configuration, including environment variables and custom mounts.
