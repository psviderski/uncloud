# uc build

Build services from a Compose file.

```
uc build [FLAGS] [SERVICE...] [flags]
```

## Options

```
  -f, --file strings      One or more Compose files to build (default compose.yaml)
  -h, --help              help for build
  -n, --no-cache          Do not use cache when building images. (default false)
  -p, --profile strings   One or more Compose profiles to enable.
  -P, --push              Push built images to the registry after building. (default false)
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.

