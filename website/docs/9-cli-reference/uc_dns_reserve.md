# uc dns reserve

Reserve a cluster domain in Uncloud DNS.

```
uc dns reserve [flags]
```

## Options

```
      --endpoint string   API endpoint for the Uncloud DNS service. (default "https://dns.uncloud.run/v1")
  -h, --help              help for reserve
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc dns](uc_dns.md)	 - Manage cluster domain in Uncloud DNS.

