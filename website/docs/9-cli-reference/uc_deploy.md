# uc deploy

Deploy services from a Compose file.

```
uc deploy [FLAGS] [SERVICE...] [flags]
```

## Options

```
  -c, --context string    Name of the cluster context to deploy to (default is the current context)
  -f, --file strings      One or more Compose files to deploy services from. (default compose.yaml)
  -h, --help              help for deploy
  -n, --no-build          Do not build images before deploying services. (default false)
  -p, --profile strings   One or more Compose profiles to enable.
      --recreate          Recreate containers even if their configuration and image haven't changed.
  -y, --yes               Auto-confirm deployment plan. Enabled by default when running non-interactively,
                          e.g., in CI/CD pipelines.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc](uc.md)	 - A CLI tool for managing Uncloud resources such as machines, services, and volumes.

