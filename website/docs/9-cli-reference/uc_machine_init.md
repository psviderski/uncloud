# uc machine init

Initialise a new cluster with a remote machine as the first member.

```
uc machine init [USER@HOST:PORT] [flags]
```

## Options

```
  -c, --context string        Name of the created context for the initialised cluster in the Uncloud config. (default "default")
      --dns-endpoint string   API endpoint for the Uncloud DNS service. (default "https://dns.uncloud.run/v1")
  -h, --help                  help for init
  -n, --name string           Assign a name to the machine.
      --network string        IPv4 network CIDR to use for machines and services. (default "10.210.0.0/16")
      --no-caddy              Don't deploy Caddy reverse proxy service to the machine.
      --no-dns                Don't reserve a cluster domain in Uncloud DNS.
      --public-ip string      Public IP address of the machine for ingress configuration. Use 'auto' for automatic detection, blank '' or 'none' to disable ingress on this machine, or specify an IP address. (default "auto")
  -i, --ssh-key string        Path to SSH private key for remote login (if not already added to SSH agent). (default "~/.ssh/id_ed25519")
      --version string        Version of the Uncloud daemon to install on the machine. (default "latest")
```

## Options inherited from parent commands

```
      --connect string          Connect to a remote cluster machine without using the Uncloud configuration file.
                                Format: [ssh://]user@host[:port] or tcp://host:port
      --uncloud-config string   Path to the Uncloud configuration file. (default "~/.config/uncloud/config.yaml")
```

## See also

* [uc machine](uc_machine.md)	 - Manage machines in an Uncloud cluster.

