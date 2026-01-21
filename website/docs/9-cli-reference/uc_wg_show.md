# uc wg show

Show WireGuard network configuration for a machine.

## Synopsis

Show the WireGuard network configuration for the machine currently connected to (or specified by the global --connect flag).

```
uc wg show [flags]
```

## Options

```
  -h, --help             help for show
  -m, --machine string   Name or ID of the machine to show the configuration for. (default is connected machine)
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc wg](uc_wg.md)	 - Inspect WireGuard network

