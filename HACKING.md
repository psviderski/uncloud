# Developing Uncloud

A short guide on setting up your local development environment.

## Prerequisites

- [mise](https://mise.jdx.dev/) (development environment setup tool)
- [Docker](https://docs.docker.com/get-docker/) (for running end-to-end tests)

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

E2e tests run inside Docker using the [`ucind`](Dockerfile) (Uncloud-in-Docker) image. Build it first:

```shell
make ucind-image
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
