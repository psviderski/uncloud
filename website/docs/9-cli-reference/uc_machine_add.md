# uc machine add

Add a remote machine to a cluster.

## Synopsis

Add a new machine to an existing Uncloud cluster.

Connection methods:
  [ssh://]user@host   - Use system 'ssh' command with full SSH config support (default, no prefix required)
  ssh+go://user@host  - Use Go's built-in SSH library

```
uc machine add [USER@]HOST[:PORT] [flags]
```

## Options

```
  -h, --help                  help for add
  -n, --name string           Assign a name to the machine.
      --no-caddy              Don't deploy Caddy reverse proxy service to the machine.
      --no-install            Skip installation of Docker, Uncloud daemon, and dependencies on the machine. Assumes they're already installed and running.
      --public-ip string      Public IP address of the machine for ingress configuration. Use 'auto' for automatic detection, blank '' or 'none' to disable ingress on this machine, or specify an IP address. (default "auto")
  -i, --ssh-key string        Path to SSH private key for remote login (if not already added to SSH agent). (default "~/.ssh/id_ed25519")
      --version string        Version of the Uncloud daemon to install on the machine. (default "latest")
      --wg-endpoint strings   WireGuard endpoint address that other machines in the cluster should use to establish WireGuard connections
                              to this machine. This doesn't change the address/port WireGuard listens on the machine.
                              Format: IP, IP:PORT, IPv6, or [IPv6]:PORT. Default port is 51820 if omitted.
                              Multiple endpoints can be specified by repeating the flag or using a comma-separated list.
                              Defaults to the auto-detected public and routable machine IPs.
  -y, --yes                   Auto-confirm prompts (e.g., resetting an already initialised machine).
                              Should be explicitly set when running non-interactively, e.g., in CI/CD pipelines. [$UNCLOUD_AUTO_CONFIRM]
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

