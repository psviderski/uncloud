# uc ctx use

Switch to a different cluster context.

## Synopsis

Switch to a different cluster context. If no context is provided, a list of available contexts will be displayed for selection.

```
uc ctx use [CONTEXT] [flags]
```

## Options

```
  -h, --help   help for use
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc ctx](uc_ctx.md)	 - Switch between different cluster contexts. Contains subcommands to manage contexts.

