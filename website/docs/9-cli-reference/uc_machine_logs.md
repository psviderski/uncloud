# uc machine logs

View systemd service logs.

## Synopsis

View logs from the specified systemd service(s) across all machines in the cluster.
Use -m to restrict to specific machines.

Supported services:
  uncloud            the Uncloud daemon
  docker             the Docker daemon
  uncloud-corrosion  the Corrosion distributed state store

If no services are specified, streams logs from the uncloud service.

```
uc machine logs [SERVICE...] [flags]
```

## Examples

```
  # View recent logs for the uncloud service.
  uc machine logs
  uc machine logs uncloud

  # Stream logs in real-time (follow mode).
  uc machine logs -f uncloud

  # View logs from multiple services.
  uc machine logs uncloud docker uncloud-corrosion

  # Show last 20 lines per machine (default is 100).
  uc machine logs -n 20 docker

  # Show all logs without line limit.
  uc machine logs -n all docker

  # View logs from a specific time range.
  uc machine logs --since 3h --until 1h30m docker

  # View logs only from specific machines.
  uc machine logs -m machine1,machine2 uncloud uncloud-corrosion
```

## Options

```
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
                                Format: [ssh://]user@host[:port], ssh+go://user@host[:port], tcp://host:port, or unix:///path/to/uncloud.sock
  -c, --context string          Name of the cluster context to use (default is the current context). [$UNCLOUD_CONTEXT]
      --uncloud-config string   Path to the Uncloud configuration file. [$UNCLOUD_CONFIG] (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in the cluster.

