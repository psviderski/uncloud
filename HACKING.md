# Developing Uncloud

A short guide on setting up your local development environment.

## Prerequisites

- [mise](https://mise.jdx.dev/) (development environment setup tool)
- [Docker](https://docs.docker.com/get-docker/) with BuildKit (for running end-to-end tests). Make sure the
  [`docker buildx`](https://docs.docker.com/build/concepts/overview/) plugin is installed.

## Setup

Uncloud uses `mise` to install and manage development tools and dependencies. It makes it easy to create reproducible
development environments on developer machines and CI.

Follow the [Installing Mise](https://mise.jdx.dev/installing-mise.html) guide to get it set up on your machine.

Mark the project as trusted and install all required tools (Go, `protoc`, `golangci-lint`, etc.) by running from the
project root:

```shell
mise trust
mise install
```

This reads [`mise.toml`](mise.toml) and installs the exact versions of tools specified there. They will be available in
your `PATH` when you are in the project directory, so you can run `go` or `golangci-lint` directly. The tools won't be
available outside the project, so they don't interfere with system packages.

## Building

Build the CLI:

```shell
go build -o uc ./cmd/uncloud
```

Or build and run the CLI with a single command:

```shell
go run ./cmd/uncloud --help
```

The Uncloud daemon (`uncloudd`) only supports Linux, so you need to cross-compile it if you're developing on macOS or
Windows:

```shell
GOOS=linux GOARCH=amd64 go build -o uncloudd ./cmd/uncloudd
```

## Testing

Run all tests (unit and e2e):

```shell
make test
```

### End-to-end tests

E2e tests run inside Docker using the [Uncloud-in-Docker](#uncloud-in-docker-ucind) image. Build it first:

```shell
mise ucind:image
```

⚠️ NOTE: You need to rebuild the `ucind` image every time you make changes to the daemon code if you want to test them
in e2e tests.

Then run the tests:

```shell
make test-e2e
```

If tests leave leftover containers, clean them up with:

```shell
make test-clean
```

### Manual testing on dev machines

After making changes to the daemon code, you may want to test them in your dev cluster. Build and install `uncloudd` on
one or multiple dev machines over SSH with:

```shell
mise run dev:install user@host1 user@host2/arm64
```

It will also restart the `uncloud.service` daemon on those machines to pick up the new version. The host format is
`[user@]host[/arch]`. The architecture of a machine defaults to `amd64` if not specified.

To stop the daemon and wipe all Uncloud data on the dev machines, run:

```shell
mise run dev:reset user@host1 user@host2
```

## Linting and formatting

Lint and format the code:

```shell
make lint-and-fix
```

## Code generation

Update the generated Go code for the machine gRPC API after modifying `.proto` files:

```shell
mise run proto
```

Regenerate mocks:

```shell
make mocks
```

## Uncloud in Docker (UCinD)

The [`ucind`](./cmd/ucind) CLI lets you run Uncloud clusters locally using Docker containers instead of real machines.
Each cluster machine runs as a Docker container connected to a shared Docker network.

Cluster machines use the `ghcr.io/psviderski/ucind:latest` Docker image by default. It may not always have the latest
changes to the `uncloudd` daemon code as it's not automatically rebuilt on CI yet.

You can rebuild it locally to pick up the latest changes, including your work in progress, and test them in a UCinD
cluster:

```shell
mise ucind:image
```

Create a cluster with 3 machines:

```shell
mise ucind cluster create -m 3
```

This also sets the new cluster as the active context, so you can interact with it using the `uc` CLI right away.

Remove the cluster when you're done:

```shell
mise ucind cluster rm
```

Sometimes crashed tests or interrupted runs can leave orphaned containers and networks behind. Clean them all up with:

```shell
mise ucind:cleanup
```
