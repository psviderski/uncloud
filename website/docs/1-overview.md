---
slug: /
---

# Overview

Uncloud makes **self-hosting web applications** across multiple machines in production dead simple.

You can connect any machines — from cloud VMs to bare metal servers (no matter where they're located) — into a secure
private network. Then run and scale multi-service and multi-container web apps and databases across your machines using
simple Docker-like commands and [Docker Compose](https://docs.docker.com/reference/compose-file/) files.

Uncloud covers all the essentials for operating apps in production without overwhelming you with the complexity of
traditional container orchestrators like Kubernetes or Swarm:

* Initial machine and network setup
* Zero-downtime rolling deployments
* Health checks and automatic restarts
* Automatic HTTPS and reverse proxy configuration
* Scaling services across multiple machines
* Cross-machine service communication without exposing ports to the internet
* DNS-based service discovery
* Load balancing
* Persistent storage

## Use cases

Some of the common use cases Uncloud is a great fit for:

- **Self-hosting and Homelabs**: Run your self-hosted apps on your own hardware. Start with a single machine and add
  more as your needs grow.
- **Outgrowing Docker Compose**: Level up your Docker Compose setup with zero-downtime deployments, replicas across
  multiple machines for improved reliability, cross-machine service communication, automated reverse proxy management,
  and more using the same Compose file.
- **Small to medium web applications**: Deploy your SaaS product, websites, or personal projects with redundancy across
  multiple machines for better reliability and your peace of mind.
- **Hybrid setups (cloud + on-prem)**: Combine cloud VMs with on-premise for cost savings and data sovereignty — all
  managed through the same interface.
- **Agencies and freelancers**: Host multiple client projects with proper isolation on shared infrastructure, optimising
  costs and resources.
- **Edge computing**: Deploy applications closer to your users for lower latency and better performance.
- **Dev/staging environments**: Spin up additional environments for development and testing that mirror production
  reusing the same Compose configuration.

## What makes Uncloud special

Here are the design decisions that make Uncloud stand out:

### Decentralised design

You can think of an Uncloud cluster as a **network of Docker hosts** (machines) that are all aware of each other. All
machines in the cluster are equal. You can connect to any of them to manage containers on any other machine in the
cluster. If a machine or part of the network goes down, the rest of the cluster keeps running.

There is no centralised control plane, so no need to worry about maintaining a quorum of machines for it. The time saved
can be better spent developing and deploying your apps instead.

### Zero-config overlay network

Uncloud automatically configures and maintains a secure **WireGuard mesh network** across your machines. It handles key
management, peer discovery, and NAT traversal without any manual configuration. This makes it easy to connect machines
from different networks and locations, such as cloud VMs, on-premise servers, or your Raspberry Pi at home.

Docker containers running on different machines get **unique IP addresses** from the cluster network so they can
**communicate directly** as if they were on a single machine without opening up any host ports to the internet.

The design and implementation were highly inspired by
Talos [KubeSpan](https://www.talos.dev/v1.10/talos-guides/network/kubespan/).

### Managed DNS service (optional)

Uncloud can provide **managed DNS records** like `<service-name>.<cluster-id>.uncld.dev` for your public
services through free [Uncloud DNS](https://github.com/psviderski/uncloud-dns) service. You can deploy a service and
instantly access it from anywhere with a proper DNS name and HTTPS without any manual DNS configuration. This makes
self-hosting much more accessible and simplifies the process of adding your own domain later.

### No complex orchestration

Uncloud operations are done using **imperative CLI commands** that have the taste of Docker and Docker Compose. The
deployment and scaling commands can output an execution plan that describes what exactly will be changed on your cluster
once you approve it. For example, what containers and volumes will be created or removed, and on which machines.

This gives you full visibility and control over every change with **immediate feedback** when something goes wrong.

### Minimal resource footprint

The Uncloud daemon consists of a couple Go and Rust binaries running alongside the Docker daemon on each machine. It
needs no more than **150 MB of RAM** and a few percent of a CPU core in small setups. This minimal overhead maximises
the system resources available for your apps.

You can run Uncloud on machines with as little as 512 MB of RAM, assuming you also need some RAM for the OS and Docker,
as well as the apps you want to run.

### Troubleshooting-friendly

When something goes wrong, you can dive straight into standard Docker containers without layers of abstraction in your
way. You can also SSH into any machine and use the regular Linux troubleshooting tools. For example, `ping` service
containers by their service names, `curl` service endpoints, or analyse traffic between containers using `wireshark`.

## Getting started

Install Uncloud CLI and deploy your first app:

* [Install Uncloud CLI](./2-getting-started/1-install-cli.md)
* [Deploy demo app](./2-getting-started/2-deploy-demo-app.md)

## Getting help

* **Discord community**: Join our [Discord server](https://discord.gg/eR35KQJhPu) for real-time discussions, support,
  and updates.
* **GitHub issues**: Report bugs or request features on our [GitHub repository](https://github.com/psviderski/uncloud).
* **Documentation**: Browse the full documentation (this website you're on) for detailed guides and references. Use
  search to find what you need quickly.
* **Newsletter**: Subscribe to the [newsletter](https://uncloud.run/#subscribe) for development updates and early
  insights.
