# Cluster storage for Caddy

The [Uncloud storage module](https://github.com/unlabs-dev/caddy-uncloud) lets Caddy instances share TLS certificates,
private keys, and ACME challenge tokens through Uncloud's cluster store. It uses distributed locks to coordinate
certificate issuance. Once one instance obtains a certificate, the others can use it too.

## Why use cluster storage?

By default, each Caddy instance stores its certificates locally. When DNS points to multiple machines or a load balancer
distributes traffic between them, an ACME challenge can reach a different instance from the one requesting the
certificate. That instance may not have the challenge token, which can delay or prevent certificate issuance.

Cluster storage is optional. The default Caddy image does not include the module, so you need to deploy an image that
includes it and configure Caddy to use it.

## Enabling cluster storage

:::warning Certificate migration

Caddy doesn't migrate existing certificates automatically when you change its storage backend. **All certificates will
need to be reissued unless you migrate your existing storage first.** Use the experimental
[`caddy storage export` and `caddy storage import`](https://caddyserver.com/docs/command-line#caddy-storage)
commands with the old and new configs to transfer the storage contents.

:::

:::info Requirements

Cluster storage requires Uncloud **v0.21.0 or newer** for both the `uc` CLI and the daemon on every cluster machine.
Check your CLI version with `uc version` and daemon versions with `uc machine ls`. Upgrade older versions before
enabling cluster storage.

:::

Create a global Caddyfile, or add `storage uncloud` to the global options in your existing one:

```caddyfile title="global.Caddyfile"
{
    storage uncloud

    # Uncomment to enable debug logs useful for troubleshooting storage operations:
    # debug
}
```

Deploy Caddy with the pre-built module image and your global config:

```shell
uc caddy deploy --image ghcr.io/unlabs-dev/caddy-uncloud:0.1.3 --caddyfile global.Caddyfile
```

See the [module README](https://github.com/unlabs-dev/caddy-uncloud#usage) for custom image builds, Compose deployment,
and additional storage options.

## Verifying storage

Check that `storage uncloud` appears in the Caddy config:

```shell
uc caddy config
```

List issued certificates in cluster storage with [`uc caddy cert ls`](../../9-cli-reference/uc_caddy_cert_ls.md):

```shell
uc caddy cert ls

ID             NAME                            ISSUER                    EXPIRES
c111e23615f7   dns.uncloud.run                 Let's Encrypt             2026-12-31 (2 months)
b5dcd2a03e1b   nginx.2t5ex2.uncld.dev          Let's Encrypt             2026-12-31 (2 months)
70707fd2fea2   test-staging.2t5ex2.uncld.dev   Let's Encrypt (staging)   2026-12-31 (2 months)
c5f9301e1c06   uncloud.run                     Let's Encrypt             2026-12-31 (2 months)
```
