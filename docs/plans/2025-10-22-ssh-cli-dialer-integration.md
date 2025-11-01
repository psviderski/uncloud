# SSH CLI Dialer Integration

**Date:** 2025-10-22
**Status:** Approved
**Branch:** exp-ssh-cli-dial-stdio

## Problem

The SSHCLIConnector currently has a non-functional `Dialer()` method that returns an error. This blocks `uc image push` functionality, which requires a proxy dialer to establish TCP connections to remote machines' unregistry services. The implementation was completed in the `exp-ssh-cli-dialer` branch and needs to be brought into the current branch.

## Goals

- Port the working Dialer() implementation from exp-ssh-cli-dialer branch
- Enable `uc image push` functionality with SSHCLIConnector
- Maintain simplicity and reuse existing SSHConnectorConfig
- Achieve feature parity with SSHConnector for image push use case

## Non-goals

- Connection pooling or caching (YAGNI for current usage patterns)
- Supporting non-TCP network types (only TCP needed for unregistry)
- Bringing the progress percent overflow fix from exp-ssh-cli-dialer

## Approach

Manual code porting from exp-ssh-cli-dialer branch rather than cherry-picking commits. This provides better control and allows adaptation to the current branch state where SSHConnectorConfig is already unified.

## Design

### Architecture

The implementation adds a new `sshCLIDialer` type that implements `proxy.ContextDialer` by spawning SSH processes with the `-W` flag for each dial operation.

**Key insight:** The existing `SSHCLIConnector.conn` field is dedicated to the gRPC dial-stdio connection. The Dialer needs to create separate, independent SSH processes for proxy connections.

### Components

**sshCLIDialer struct:**

```go
type sshCLIDialer struct {
    config SSHConnectorConfig
}
```

Embeds the entire config to avoid field duplication. Creates new SSH processes on demand.

**DialContext method:**

```go
func (d *sshCLIDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
    // 1. Validate network type (only TCP supported)
    // 2. Build SSH command args using buildDialArgs()
    // 3. Execute: ssh -W address user@host via commandconn.New()
    // 4. Return net.Conn or error
}
```

**buildDialArgs helper:**

```go
func (d *sshCLIDialer) buildDialArgs(address string) []string {
    // Constructs: [-o ConnectTimeout=5] [-p PORT] [-i KEY] [-W address] [user@host]
    // Similar pattern to SSHCLIConnector.buildSSHArgs but uses -W flag
}
```

**Updated Dialer() method:**

```go
func (c *SSHCLIConnector) Dialer() (proxy.ContextDialer, error) {
    if c.config == (SSHConnectorConfig{}) {
        return nil, fmt.Errorf("SSH connector not configured")
    }
    return &sshCLIDialer{config: c.config}, nil
}
```

### Flow

When `uc image push` executes:

1. Client calls `connector.Dialer()` → receives `sshCLIDialer` instance
2. Image push calls `dialer.DialContext(ctx, "tcp", "10.210.1.1:5000")`
3. sshCLIDialer spawns new SSH process: `ssh -o ConnectTimeout=5 [-options] -W 10.210.1.1:5000 user@host`
4. Returns `net.Conn` connected to unregistry
5. Image push uses connection for docker push, then closes it
6. SSH process terminates when connection closes

**Concurrent operations:** Multiple parallel image pushes spawn multiple independent SSH processes. No connection pooling needed for current usage patterns.

### SSH -W Flag

The `-W host:port` flag forwards stdin/stdout to the specified address over the SSH secure channel. This provides the same functionality as the Go SSH client's `DialContext()` but using the CLI.

From `man ssh`:
```
-W host:port
    Requests that standard input and output on the client be forwarded to
    host on port over the secure channel.
```

### Error Handling

1. **Unconfigured connector**: Return error if `config == (SSHConnectorConfig{})`
2. **Unsupported network type**: Return error if `network != "tcp"`
3. **SSH failures**: Propagate errors from `commandconn.New()` (auth failures, unreachable host, network issues)
4. **Context cancellation**: Automatically handled by `commandconn.New()`

**Connection cleanup:**
- Caller receives `net.Conn` and must call `Close()`
- SSH process terminates when connection closes (stdin/stdout EOF)
- No explicit process tracking needed

## Implementation Tasks

1. Add `sshCLIDialer` struct to `pkg/client/connector/sshcli.go`
2. Implement `DialContext()` method with network type validation
3. Implement `buildDialArgs()` helper method
4. Update `SSHCLIConnector.Dialer()` to return `sshCLIDialer` instance
5. Port comprehensive tests to `pkg/client/connector/sshcli_test.go`:
   - Test `buildDialArgs()` with various configurations (ports, key paths)
   - Test `DialContext()` error for unsupported network types
   - Test `Dialer()` error when connector not configured

## Testing Strategy

**Unit tests:**
- `buildDialArgs()` with standard port, custom port, with/without key
- `DialContext()` returns error for non-TCP network types
- `Dialer()` validates config is present

**Manual validation:**
- Test `uc image push` with SSHCLIConnector
- Verify image successfully pushed to remote unregistry
- Confirm no SSH process leaks after completion

## References

- exp-ssh-cli-dialer branch commits: 0d53600, 1d8bb77
- Existing design doc: docs/plans/2025-10-21-sshcli-dialer-design.md
- SSH man page: `man ssh` (see -W flag)
