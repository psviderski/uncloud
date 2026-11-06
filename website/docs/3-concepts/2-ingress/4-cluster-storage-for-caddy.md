# Cluster storage for Caddy

Starting with Uncloud [v0.22.0](https://github.com/psviderski/uncloud/releases/tag/v0.22.0), Caddy uses cluster storage
by default.

The [Uncloud storage module](https://github.com/unlabs-dev/caddy-uncloud) lets Caddy instances share TLS certificates,
private keys, and ACME challenge tokens through Uncloud's cluster store.

This matters when you run Caddy on multiple machines and distribute traffic between them using DNS or a load balancer.
In this case, an ACME challenge can reach a different Caddy instance from the one requesting the certificate. Cluster
storage lets any Caddy instance read the challenge token from the shared cluster store and complete the ACME challenge.
It uses distributed locks to coordinate certificate issuance. Once one instance obtains a certificate, the others can
use it too.

## Upgrading from earlier versions

Starting with Uncloud v0.22.0, `uc caddy deploy` deploys the Uncloud Caddy image ([
`ghcr.io/unlabs-dev/caddy-uncloud`](https://github.com/unlabs-dev/caddy-uncloud/pkgs/container/caddy-uncloud)) with
cluster storage enabled by default.

Earlier versions deployed the official [`caddy`](https://hub.docker.com/_/caddy) image with local certificate storage.
Upgrading Uncloud does not change the running Caddy service. Redeploy it to switch to the new default image and cluster
storage.

Before you deploy, upgrade the [`uc` CLI](../../2-getting-started/1-install-cli.md) and the daemon on all machines to
v0.22.0 or newer. Follow the [upgrade instructions](https://github.com/psviderski/uncloud/releases/latest) for daemon
upgrades, including any required migrations from older releases. Check current versions with `uc version` and `uc
machine ls`.

The first `uc caddy deploy` after the upgrade doesn't know the values you set with earlier versions of `uc`. It replaces
your image, global config, and machine list with the defaults unless you pass them as flags.

If you already use `storage uncloud`, your certificates stay where they are, so you only need to run `uc caddy deploy`.

### Quick upgrade

Caddy does not copy certificates when you change storage backends. Unless you migrate them first, Caddy will reissue
them. HTTPS may fail for those domains until that finishes, and a large number of certificates can hit
[Let's Encrypt rate limits](https://letsencrypt.org/docs/rate-limits/).

Run the command below if you don't mind reissuing all certificates:

```shell
uc caddy deploy
```

### Migrating certificates

Follow these steps to keep your existing certificates and avoid HTTPS failures:

1. Deploy the new image but stay on local storage. Use your existing global config if you have one, or an empty file:

   ```shell
   touch local.Caddyfile
   uc caddy deploy --caddyfile local.Caddyfile
   ```

2. Find the ID of the Caddy container on each machine:

   ```shell
   uc inspect caddy
   ```

3. Copy the certificates to cluster storage by running this command for each container ID. Each machine stores its own
   certificates, so don't skip any of them. These
   [`caddy storage`](https://caddyserver.com/docs/command-line#caddy-storage) commands are experimental.

   ```shell
   uc exec -T --container CONTAINER_ID caddy sh -c '
     printf "{\n\tstorage uncloud\n}\n" > /tmp/Caddyfile.cluster
     caddy storage export --config /etc/caddy/Caddyfile --output - |
       caddy storage import --config /tmp/Caddyfile.cluster --input -
     rm /tmp/Caddyfile.cluster
   '
   ```

4. Check that all your certificates are in cluster storage:

   ```shell
   uc caddy cert ls
   ```

5. Switch Caddy to cluster storage. If you have a custom global config, add `storage uncloud` to it and pass it with
   `--caddyfile global.Caddyfile` instead. The command warns that all certificates will be reissued. You can ignore this
   because you've already copied them.

   ```shell
   uc caddy deploy --caddyfile ""
   ```

6. Check that the Caddy config includes `storage uncloud` and doesn't show any config load warnings:

   ```shell
   uc caddy config
   ```

### Upgrading a Compose deployment

If you manage Caddy with Compose, keep using `uc deploy` and update your Compose file to match
[Compose example](3-managing-caddy.md#managing-caddy-with-compose). To keep your certificates, follow the
[migration steps](#migrating-certificates) with these changes:

- In step 1, update the image and add the `/run/uncloud/api:/run/uncloud/api:ro` volume mount but leave out
  `storage uncloud` in your global Caddy config (`x-caddy`). Then run `uc deploy`.
- In step 5, add `storage uncloud` to your global Caddy config, then run `uc deploy`.

## Keeping local storage

To keep local storage, pass a custom Caddyfile without `storage uncloud` using `--caddyfile`. The default Uncloud Caddy
image supports local storage too. If you use the official `caddy` image, you must also provide a Caddyfile without
`storage uncloud`.

Deployments using `uc caddy deploy` keep the `/data` mount so local certificates survive container updates. For Compose
deployments using local storage, keep or add `/var/lib/uncloud/caddy:/data`.
