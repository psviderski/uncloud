# uc dns

Manage cluster domain in Uncloud DNS.

## Synopsis

Manage cluster domain in Uncloud DNS.
DNS commands allow you to reserve or release a unique 'xxxxxx.uncld.dev' domain for your cluster. When reserved, Caddy service deployments will automatically update DNS records to route traffic to the services in the cluster.

## Options

```
  -h, --help   help for dns
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.
* [uc dns release](uc_dns_release.md)	 - Release the reserved cluster domain.
* [uc dns reserve](uc_dns_reserve.md)	 - Reserve a cluster domain in Uncloud DNS.
* [uc dns show](uc_dns_show.md)	 - Print the cluster domain name.

