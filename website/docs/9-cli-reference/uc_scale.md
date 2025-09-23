# uc scale

Scale a replicated service by changing the number of replicas.

## Synopsis

Scale a replicated service by changing the number of replicas. Scaling down requires confirmation.

```
uc scale SERVICE REPLICAS [flags]
```

## Options

```
  -c, --context string   Name of the cluster context. (default is the current context)
  -h, --help             help for scale
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.

