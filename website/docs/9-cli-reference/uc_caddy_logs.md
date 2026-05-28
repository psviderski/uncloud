# uc caddy logs

View caddy logs.

## Synopsis

View caddy logs.

This calls "uc logs caddy", see "uc logs" for the documention.


```
uc caddy logs [flags]
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

* [uc caddy](uc_caddy.md)	 - Manage Caddy reverse proxy service.

