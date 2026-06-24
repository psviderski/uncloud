# uc dns set

Set a cluster domain directly in the cluster.

## Synopsis

Set a cluster domain directly in the cluster, bypassing Uncloud DNS. This assumes the DNS is externally set up.

```
uc dns set DOMAIN_NAME [flags]
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

* [uc dns](uc_dns.md)	 - Manage cluster domain in Uncloud DNS.

