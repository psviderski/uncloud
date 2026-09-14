# uc destroy

Destroy services from a Compose file.

## Synopsis

Destroy services from a Compose file.

Destroy removes all containers of the specified service(s) across all machines in the cluster.

See "uc service remove".

```
uc destroy [FLAGS] [SERVICE...] [flags]
```

## Options

```
  -f, --file strings      One or more Compose files to destroy services from. (default compose.yaml)
  -h, --help              help for destroy
  -p, --profile strings   One or more Compose profiles to enable.
  -y, --yes               Auto-confirm deployment plan. Should be explicitly set when running non-interactively,
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

