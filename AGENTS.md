# AGENTS.md - Uncloud Project Guide

This document provides comprehensive information about the Uncloud project for AI assistants to understand the codebase, architecture, and development practices.

## Project Overview

**Uncloud** is a lightweight clustering and container orchestration tool that enables deployment and management of web applications across cloud VMs and bare metal servers. It creates a secure WireGuard mesh network between Docker hosts and provides automatic service discovery, load balancing, HTTPS ingress, and simple CLI commands for application management.

### Key Characteristics

- **Language**: Go
- **Architecture**: Decentralized, no control plane
- **Target**: Self-hosted infrastructure without Kubernetes complexity
- **License**: View LICENSE file for details
- **Status**: Active development, not yet ready for production

## Core Features

### 🏗️ Infrastructure

- **Multi-machine deployment**: Combine cloud VMs, dedicated servers, and bare metal
- **Zero-config networking**: Automatic WireGuard mesh with NAT traversal
- **Decentralized design**: No central control plane, all machines are equal
- **Service discovery**: Built-in DNS server resolves service names to container IPs

### 🚀 Application Management

- **Docker Compose compatibility**: Uses familiar Docker Compose format
- **Zero-downtime deployments**: Rolling updates without service interruption
- **Automatic HTTPS**: Caddy reverse proxy with Let's Encrypt integration
- **Managed DNS**: Free `*.xxxxxx.uncld.dev` subdomains via Uncloud DNS service
- **Cross-machine scaling**: Run containers across multiple machines

### 🔧 Developer Experience

- **Docker-like CLI**: Familiar commands (`uc` binary)
- **Imperative operations**: Direct commands vs. declarative state reconciliation
- **Remote management**: Control entire infrastructure via SSH to any machine
- **Minimal overhead**: ~150MB RAM footprint per machine

## Architecture

### Core Components

1. **CLI (`uc`)** - Main user interface for cluster management
2. **Daemon (`uncloudd`)** - Machine daemon running on each node
3. **Corrosion** - Distributed SQLite database for cluster state (Fly.io project)
4. **Caddy** - Reverse proxy for HTTPS termination and routing
5. **WireGuard** - Secure mesh networking between machines

### Network Architecture

- Each machine gets unique subnet (e.g., `10.210.0.0/24`, `10.210.1.0/24`)
- Containers get cluster-unique IPs for direct communication
- Automatic peer discovery and key management
- NAT traversal for machines behind firewalls

### State Management

- **CRDT-based distributed storage** using Corrosion
- **Eventually consistent** state across all machines
- **Gossip protocol** (Serf) for state propagation
- **No quorum requirements** - partial network splits remain functional

## Project Structure

### Key Directories

- **`cmd/`**: Contains main applications

  - `uncloud/`: CLI tool with subcommands for machine, service, volume management
  - `uncloudd/`: Daemon that runs on each machine
  - `ucind/`: Development cluster management for testing

- **`internal/`**: Internal implementation packages

  - `cli/`: Command-line interface logic
  - `machine/`: Machine lifecycle and state management
  - `daemon/`: Daemon implementation and gRPC services
  - `dns/`: Internal DNS server for service discovery

- **`pkg/`**: Public API packages for external use

  - `api/`: Core API types and definitions
  - `client/`: Client libraries for interacting with Uncloud

- **`experiment/`**: Experimental features and prototypes
- **`scripts/`**: Installation and utility scripts
- **`test/`**: Test suites and test infrastructure
- **`website/`**: Documentation website (Docusaurus)

  - `landing/`: Landing page
  - `docs/`: User documentation

- **`misc/`**: Design documents and guides

## Key Technologies

### Core Dependencies

```go
// Networking and orchestration
github.com/docker/docker                   // Docker API client
github.com/docker/compose/v2               // Docker Compose integration
golang.zx2c4.com/wireguard                 // WireGuard implementation
github.com/hashicorp/serf                  // Gossip protocol

// State management
github.com/ipfs/go-ds-crdt                 // CRDT distributed storage
github.com/dgraph-io/badger/v3             // Embedded database

// Web proxy
github.com/caddyserver/caddy/v2            // HTTP server and reverse proxy

// CLI and UX
github.com/spf13/cobra                     // CLI framework
github.com/charmbracelet/huh               // Interactive forms

// gRPC and networking
google.golang.org/grpc                     // gRPC framework
github.com/siderolabs/grpc-proxy           // gRPC proxy for forwarding
```

## Development Workflow

### Build and Development

```bash
# Build binaries
go build -o uncloud ./cmd/uncloud
go build -o uncloudd ./cmd/uncloudd
```

### Key Make Targets

- `proto`: Generate protobuf code
- `ucind-cluster`: Create development cluster
- `update-dev`: Deploy to development machines
- `demo-reset`: Reset demo environment
- `fmt`: Format code
- `test`: Run all tests
- `lint`: Lint the code using golangci-lint
- `lint-and-fix`: Lint the code and fix issues whenever possible

## CLI Commands Structure

The `uc` CLI provides these main command groups:

### Machine Management

```bash
uc machine init <user@host>     # Initialize new cluster
uc machine add <user@host>      # Add machine to cluster
uc machine ls                   # List machines
uc machine rm <name>            # Remove machine
```

### Service Management

```bash
uc run <image>                  # Run container from image
uc deploy                       # Deploy from compose.yaml
uc scale <service> <count>      # Scale service replicas
uc ls                           # List services
uc rm <service>                 # Remove service
```

### Context and Connectivity

```bash
uc context ls                   # List available contexts
uc context use <name>           # Switch context
```

### Global Flags

- `--connect`: Connect to remote machine directly, without a config file
- `--uncloud-config`: Override config file path

## Development Guidelines

### Code Organization

- **Package naming**: Use clear, descriptive names
- **Error handling**: Wrap errors with context using `fmt.Errorf`
- **Logging**: Use structured logging with levels
- **gRPC**: Services defined in `internal/machine/api/pb/`

### Testing

- Test files and locations
  - Unit tests alongside source files (`*_test.go`)
  - Integration tests in `test/e2e/`
  - Test fixtures in `test/fixtures/`
- Use table driven tests whenever possible

### Dependencies

- Prefer standard library when possible
- Pin versions in `go.mod`
- Document rationale for external dependencies

### Configuration

- Support environment variables for key settings
- Validate configuration early
- Provide sensible defaults

## Troubleshooting and Debugging

### Common Issues

- **Networking**: Check WireGuard status, iptables rules
- **DNS**: Verify service discovery resolution
- **Containers**: Use standard Docker debugging tools
- **State sync**: Check Corrosion logs for replication issues

### Debugging Tools

- Standard Linux networking tools (`ping`, `traceroute`, `wireshark`)
- Docker commands (`docker ps`, `docker logs`)
- SSH access to machines for direct inspection
- gRPC debugging tools

### Logs and Monitoring

- Systemd services (getting logs via `journalctl -u SERVICE_NAME`)
  - `uncloud` -- Uncloud daemon
  - `uncloud-corrosion` -- Corrosion process
- Machine daemon logs
- Container logs via Docker

## File Patterns and Conventions

### Important Files to Understand

- `cmd/uncloud/main.go`: CLI entry point and command structure
- `internal/cli/cli.go`: CLI implementation and configuration
- `internal/machine/machine.go`: Core machine management
- `pkg/api/`: Public API definitions
- `misc/design.md`: Architecture and design philosophy
- `README.md`: Repository README

### Configuration Files

- `go.mod/go.sum`: Go dependency management
- `Makefile`: Build and development tasks
- `Dockerfile`: Container build instructions forUncloud-in-Docker (used for testing)

This document should help AI assistants understand the project structure, make informed suggestions, and contribute effectively to the Uncloud codebase.

## Documentation

Instructions when generating documentation:

- Use conversational language – write as if you were speaking to a friend.
- Keep sentences simple, optimize for clarity and understanding.
- Place the subject before the action whenever possible. Example: prefer “The function loads data” over “Data is loaded by the function.”
