# Uncloud design

Uncloud aims to provide a solution for deploying and running containerized applications and services across various
platforms — from cloud VPSs to Raspberry Pis and powerful servers — or any combination of them.

While existing prominent PaaS solutions like https://coolify.io/ and https://dokploy.com/ use a single machine or Docker
Swarm as their underlying infrastructure, Uncloud takes a different approach. It aims to offer a multi-host experience
similar to the serverless solutions provided by cloud providers but on users' own machines. Users should be able to add
machines as compute resources without worrying about control plane high availability and cluster management. For
example, a user could combine a cloud VPS with a spare Raspberry Pi to create a unified computing environment.

Uncloud aims to replace the concept of a "cluster" with a "network" of machines. It's similar to Tailscale's design
where users can add machines to a network to enable secure communication between them. The primary challenges in this
design are:

1. Building such a network
2. Orchestrating containers across it
3. Exposing services to the internet

## Network of machines

While Tailscale is an obvious solution for establishing an overlay network, it has some drawbacks:

1. Users must register an account and generate a key for each machine.
2. Every container requires a Tailscale proxy to communicate with other machines and containers.

The Tailscale subnet router feature could potentially be used to route traffic from the machine to the internal Docker
network.

As an alternative, we can configure a simple flat WireGuard mesh network during the machine setup process. For example:

| CIDR            | Description                                              |
|-----------------|----------------------------------------------------------|
| `10.210.0.0/16` | The entire WireGuard mesh network                        |
| `10.210.X.0/24` | `/24` subnet assigned to machine `X`                     |
| `10.210.X.1/32` | Machine `X` address is the first address from the subnet |
| `10.210.X.Y/32` | Container `Y` address running on machine `X`             |

Testing has shown that inter-container and machine routing works well when `/24` is assigned to the Docker network
bridge, while the WireGuard interface is configured with just the `/32` machine address.

This setup allows containers to communicate with each other on the same machine or across machines without address
translation. This design also enables the future implementation of [ACLs](https://tailscale.com/kb/1018/acls) and
security groups to restrict traffic between machines and containers if needed.

The peer discovery and NAT traversal techniques for the WireGuard mesh are heavily inspired by the
Talos [KubeSpan](https://www.talos.dev/v1.7/talos-guides/network/kubespan/) design.

## Orchestration

The main question that drives the design of the orchestration system is can we build a system that doesn't require a
centralised control plane? The short answer is yes, we can. However, it's not easy as decentralised systems are
inherently more complex than centralised ones.

The existing solutions such as Kubernetes, Docker Swarm, and Nomad use a centralised control plane to provide a single
point of entry for the user to interact with the cluster. I'm envisioning a system where all machines are equal, yet
each one can represent the entire cluster and receive commands from the user. Where a user can be a CLI tool or a web
interface that can run on any machine in the network. In case of network partitioning, the user should continue to be
able to interact with each partition separately to manage services running on them. The system should be able to
reconcile the state of the network when the partition is healed. Such an architecture favors Availability and Partition
tolerance (AP) over Consistency and Availability (CA) in the [CAP theorem](https://en.wikipedia.org/wiki/CAP_theorem).

### Shared state

One of the ways to achieve this is to store the entire shared state on every machine and keep them all in sync. A couple
of techniques can be used to assist with this:
a [Conflict-Free Replicated Data Type](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) (CRDT) and a
peer-to-peer [gossip protocol](https://en.wikipedia.org/wiki/Gossip_protocol).

By composing the state using CRDT data structures allows any machine to modify its own copy of the state independently,
concurrently, and without coordination with other machines. The CRDT automatically resolves conflicts that might arise
from concurrent updates. Although the machines may have different states at any given time, they're guaranteed to
eventually converge to the *same* state. The tradeoff is that the *same* state doesn't always mean the intended one from
each user's point of view.

There is a distributed key-value store implementation in Go ([ipfs/go-ds-crdt](https://github.com/ipfs/go-ds-crdt)) from
the IPFS ecosystem that uses
[Merkle CRDTs](https://research.protocol.ai/publications/merkle-crdts-merkle-dags-meet-crdts/psaras2020.pdf). It's
independent of the permanent storage implementation and the transport layer for broadcasting and receiving updates from
peers.

The gossip protocol can be used to propagate state updates across the network. There is a great project from HashiCorp
called [Serf](https://www.serf.io/) that provides a gossip protocol implementation in addition to cluster membership,
failure detection, and basic orchestration that is decentralized, fault-tolerant and highly available. It's used in
Nomad and Consul internally.

[`experiment/serf_crdt.go`](../experiment/serf_crdt.go) is an experimental distributed key-value store
using [BadgerDB](https://github.com/dgraph-io/badger) as the persistent storage for
[ipfs/go-ds-crdt](https://github.com/ipfs/go-ds-crdt) and Serf as the gossip protocol that uses its custom user events
and queries features. The implementation is very inefficient but the results are promising. A couple of machines are
able to replicate changes to the store across the internet in a fraction of a second and converge their copies if they
get out of sync. Although, the convergence is quite slow at the moment due to the inefficient broadcast and query
implementation.

There is a hypothesis that in practice there shouldn't be much churn and conflicts in the state of relatively small
deployments. The updates should be propagated quickly enough thus the eventually consistent behaviour should be
acceptable.

### Container scheduling

There are two fundamental approaches to design an orchestration system: declarative and imperative. I believe that
Uncloud should use a hybrid approach that favours the imperative over the declarative one where possible.

The imperative approach allows the errors to be handled in a more predictable way. While the declarative approach allows
to decouple components of the system by using a shared state and events for communication.

For example, the user can run a command to start a container on a specific machine. The command can call the target
machine directly to start the container, handle the errors accordingly, and return the result to the user.
Alternatively, the command can update the local state declaring that a container should be started on a specific machine
and wait until it's propagated to the target machine and reconciled. Arguably, the latter approach is more complex, less
predictable, and has more edge cases to handle.

However, asynchronously updating the configuration of DNS servers or reverse proxies in accordance to the state changes
caused by started or stopped containers is likely a more reliable approach than doing it imperatively.

Not sure if we really need this yet, but more complex scenarios like running and watching a replica set of containers
might require a sort of coordination between machines. I believe this can be achieved by using consensus and leader
election algorithms on demand.

Maybe for now we should only aim for a simpler and more static container orchestrator where container scheduling can
only be initiated by a user. Docker on each machine will ensure that the containers are running and restarted on
failures, of course. But they won't be moved to other machines automatically.

## Service discovery

Service discovery using DNS has its own drawbacks but it's likely the simplest way to implement it. Like in Docker and
Docker Swarm, the Uncloud agent on every machine will expose an internal DNS server to the machine itself and the
containers running on it. The DNS server will resolve machine, container, and service names to their respective IP
addresses within the mesh network.

DNS servers will watch running containers through the shared state and update their DNS records accordingly. The
following table is an example list of available DNS names:

| DNS name                                           | Resolves to                                                                |
|----------------------------------------------------|----------------------------------------------------------------------------|
| `<machine-name>.machine.internal`                  | Mesh IP of the machine. Machine names must be unique within the org        |
| `<container-name>.<machine-name>.machine.internal` | Mesh IP of the container on the machine                                    |
| `<service-name>.internal`                          | Mesh IPs of all containers for the service                                 |
| `lb.<service-name>.internal`                       | Virtual IP / LB IP that balances traffic to all containers for the service |

A proper thinking about the name design is needed. Most likely we need to namespace them by project or environment name
to allow for multiple instances of the same service and avoid conflicts.

Virtual IPs for services using IPVS, ip/nftables, or eBPF can be implemented later if we see fit.

## Ingress

Services can be exposed to the internet by running a reverse proxy on a machine(s) that listens on the public IP
address. A user should be able to control which machine(s) should run a reverse proxy by assigning an appropriate role
to them.

Traefik or Caddy can be used as a reverse proxy that can automatically discover services running on the network using
the internal DNS server. They both support automatic TLS certificate generation and renewal using Let's Encrypt.

In case of network partitioning, DNS servers should adjust their records in accordance with what containers are
available in their partition. Consequently, the reverse proxy should adjust its configuration to route traffic to only
the available containers by resolving the updated DNS records.
