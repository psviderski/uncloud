# uc dns set

Set or unset an externally managed cluster domain (EXPERIMENTAL).

## Synopsis

EXPERIMENTAL: Set the cluster domain used to generate ingress hostnames for services.
Configure wildcard DNS records for this domain with your DNS provider. Uncloud will not create, verify, update, or delete external DNS records.
Pass an empty string to unset a manually set domain. Use 'uc dns release' to release a domain reserved in Uncloud DNS. Setting or unsetting the domain does not change existing service hostnames.

```
uc dns set DOMAIN_NAME [flags]
```

## Examples

```
  uc dns set apps.example.com
  uc dns set ""
```

## Options

```
  -h, --help   help for set
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc dns](uc_dns.md)	 - Manage the cluster domain.

