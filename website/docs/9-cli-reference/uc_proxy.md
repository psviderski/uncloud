# uc proxy

Proxy a service port to a local port.

## Synopsis

Proxy a service port in the cluster to a local port on this machine.

If the service runs multiple containers, the command connects to the first running and healthy one.
If you don't provide a local port, the command picks a random one.

The connection stays open for as long as the command runs.

```
uc proxy SERVICE [LOCAL_PORT:]REMOTE_PORT [flags]
```

## Options

```
  -h, --help   help for proxy
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

