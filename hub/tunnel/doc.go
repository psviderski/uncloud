// Package tunnel implements reverse tunnels between machines and Uncloud Hub. A machine opens a tunnel as an outbound
// gRPC stream to Hub and authenticates with its WireGuard key. The stream then carries a gRPC connection in the
// opposite direction where Hub is the client and the machine serves the Uncloud API.
package tunnel
