// Package distlock provides distributed, automatically renewed leases across independent nodes.
//
// Its quorum and lease semantics are based on the Redlock algorithm described at
// https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/. The core package is independent of storage
// and network transport and does not require Redis. Applications that communicate with remote nodes over gRPC can use
// the grpc subpackage to expose and call node-local lease operations.
//
// A Cluster must return every node in the lock group, including temporarily unavailable nodes, because every node
// counts toward quorum. Changing the node set is unsafe if a new quorum can be disjoint from an earlier quorum while
// leases acquired from the earlier node set may still be valid. Callers must stop protected work when the context
// returned by Lease.Context is done.
package distlock
