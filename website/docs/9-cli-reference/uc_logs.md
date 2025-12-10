# uc logs

View service logs.

## Synopsis

View logs from all replicas of the specified service(s) across all machines in the cluster.

If no services are specified, streams logs from all services defined in the Compose file
(compose.yaml by default or the file(s) specified with --file).

```
uc logs [SERVICE...] [flags]
```

## Examples

```
  # View recent logs for a service.
  uc logs web

  # Stream logs in real-time (follow mode).
  uc logs -f web

  # View logs from multiple services.
  uc logs web api db

  # View logs from all services in compose.yaml.
  uc logs

  # Show last 20 lines per replica (default is 100).
  uc logs -n 20 web

  # Show all logs without line limit.
  uc logs -n all web

  # View logs from a specific time range.
  uc logs --since 3h --until 1h30m web

  # View logs only from replicas running on specific machines.
  uc logs -m machine1,machine2 web api
```

## Options

```
      --file strings      One or more Compose files to load service names from when no services are specified. (default compose.yaml)
  -f, --follow            Continually stream new logs.
  -h, --help              help for logs
  -m, --machine strings   Filter logs by machine name or ID. Can be specified multiple times or as a comma-separated list.
      --since string      Show logs generated on or after the given timestamp. Accepts relative duration, RFC 3339 date, or Unix timestamp.
                          Examples:
                            --since 2m30s                      Relative duration (2 minutes 30 seconds ago)
                            --since 1h                         Relative duration (1 hour ago)
                            --since 2025-11-24                 RFC 3339 date only (midnight using local timezone)
                            --since 2024-05-14T22:50:00        RFC 3339 date/time using local timezone
                            --since 2024-01-31T10:30:00Z       RFC 3339 date/time in UTC
                            --since 1763953966                 Unix timestamp (seconds since January 1, 1970)
  -n, --tail string       Show the most recent logs and limit the number of lines shown per replica. Use 'all' to show all logs. (default "100")
      --until string      Show logs generated before the given timestamp. Accepts relative duration, RFC 3339 date, or Unix timestamp.
                          See --since for examples.
      --utc               Print timestamps in UTC instead of local timezone.
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

