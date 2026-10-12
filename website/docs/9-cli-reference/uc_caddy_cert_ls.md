# uc caddy cert ls

List certificates in cluster storage for Caddy.

## Synopsis

List certificates stored in the Uncloud cluster storage for Caddy.

Caddy must use the Uncloud storage module (https://github.com/unlabs-dev/caddy-uncloud)
configured with 'storage uncloud' in the global options for its certificates to appear here.

This inventory does not check whether Caddy currently serves or trusts a certificate.

```
uc caddy cert ls [flags]
```

## Options

```
  -h, --help             help for ls
  -m, --machine string   Name or ID of the machine to read certificate storage from. (default is connected machine)
  -o, --output string    Output format: 'json' or empty for a human-readable table.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc caddy cert](uc_caddy_cert.md)	 - Inspect certificates in cluster storage for Caddy.

