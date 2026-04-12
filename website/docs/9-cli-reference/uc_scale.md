# uc scale

Scale a replicated service by changing the number of replicas.

## Synopsis

Scale a replicated service by changing the number of replicas.

```
uc scale SERVICE REPLICAS [flags]
```

## Options

```
  -h, --help   help for scale
  -y, --yes    Auto-confirm scaling plan. Should be explicitly set when running non-interactively,
               e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.

