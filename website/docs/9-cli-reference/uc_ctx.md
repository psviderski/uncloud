# uc ctx

Switch between different cluster contexts. Contains subcommands to manage contexts.

```
uc ctx [flags]
```

## Options

```
  -h, --help   help for ctx
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.
* [uc ctx ls](uc_ctx_ls.md)	 - List available cluster contexts.
* [uc ctx use](uc_ctx_use.md)	 - Switch to a different cluster context.

