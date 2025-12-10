# uc deploy

Deploy services from a Compose file.

```
uc deploy [FLAGS] [SERVICE...] [flags]
```

## Options

```
      --build-arg stringArray   Set a build-time variable for services. Used in Dockerfiles that declare the variable with ARG.
                                Can be specified multiple times. Format: --build-arg VAR=VALUE
      --build-pull              Always attempt to pull newer versions of base images before building service images.
  -f, --file strings            One or more Compose files to deploy services from. (default compose.yaml)
  -h, --help                    help for deploy
      --no-build                Do not build new images before deploying services.
      --no-cache                Do not use cache when building images.
  -p, --profile strings         One or more Compose profiles to enable.
      --recreate                Recreate containers even if their configuration and image haven't changed.
  -y, --yes                     Auto-confirm deployment plan. Should be explicitly set when running non-interactively,
                                e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]
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

