# uc rm

Remove one or more services.

## Synopsis

Remove one or more services.

The volumes used by the services are preserved and should be removed separately
with 'uc volume rm'. Anonymous Docker volumes (automatically created from VOLUME
directives in image Dockerfiles) are automatically removed with their containers.

```
uc rm SERVICE [SERVICE...] [flags]
```

## Options

```
  -h, --help   help for rm
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

