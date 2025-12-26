# uc service exec

Execute a command in a running service container.

## Synopsis

Execute a command (interactive shell by default) in a running container within a service.
If the service has multiple replicas and no container ID is specified, the command will be executed in a random container.
	

```
uc service exec [OPTIONS] SERVICE [COMMAND ARGS...] [flags]
```

## Examples

```

  # Start an interactive shell ("bash" or "sh" will be tried by default)
  uc exec web-service

  # Start an interactive shell with explicit command
  uc exec web-service /bin/zsh

  # List files in the specific container of the service; --container accepts full ID or a (unique) prefix
  uc exec --container d792e web-service ls -la

  # Pipe input to a command inside the service container
  cat backup.sql | uc exec -T db-service psql -U postgres mydb

  # Run a task in the background (detached mode)
  uc exec -d web-service /scripts/cleanup.sh
```

## Options

```
      --container string   ID of the container to exec into. Accepts full ID or a unique prefix (default is the random container of the service)
  -d, --detach             Detached mode: run command in the background
  -h, --help               help for exec
  -T, --no-tty             Disable pseudo-TTY allocation. By default 'uc exec' allocates a TTY when connected to a terminal.
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file. [$UNCLOUD_CONNECT]
                                Format: [ssh://]user@host[:port], ssh+cli://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc service](uc_service.md)	 - Manage services in the cluster.

