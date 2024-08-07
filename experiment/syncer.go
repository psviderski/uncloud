package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ipfs/boxo/datastore/dshelp"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ipld "github.com/ipfs/go-ipld-format"
	"log/slog"
)

// Implements the DAGService interface.
// TODO: implement SessionDAGService to optimize node fetching.
// TOOD: persistentSerfDAG?
type dagSyncer struct {
	// Persistent storage for the nodes.
	store     ds.Datastore
	namespace ds.Key
}

func newDAGSyncer(store ds.Datastore, namespace ds.Key) *dagSyncer {
	return &dagSyncer{
		store:     store,
		namespace: namespace,
	}
}

func (d *dagSyncer) Get(ctx context.Context, cid cid.Cid) (ipld.Node, error) {
	slog.Debug("Getting node", "cid", cid)
	nodeBytes, err := d.store.Get(ctx, d.nodeKey(cid))
	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			// TODO: try to retrieve the node from the peers.
			return nil, ipld.ErrNotFound{Cid: cid}
		}
		return nil, fmt.Errorf("get node %s from local store: %w", cid, err)
	}
	protoNode, err := dag.DecodeProtobuf(nodeBytes)
	if err != nil {
		return nil, fmt.Errorf("decode node from protobuf: %w", err)
	}
	// CID is lazily computed from the node content. Ensure the node uses CIDv1.
	if err = protoNode.SetCidBuilder(dag.V1CidPrefix()); err != nil {
		return nil, fmt.Errorf("set CIDv1 on node: %w", err)
	}
	slog.Debug("Retrieved node from local store", "cid", protoNode.Cid())

	return protoNode, nil
}

func (d *dagSyncer) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	//TODO implement me
	panic("implement me")
}

func (d *dagSyncer) Add(ctx context.Context, node ipld.Node) error {
	slog.Debug("Adding node", "cid", node.Cid())
	protoNode, ok := node.(*dag.ProtoNode)
	if !ok {
		return fmt.Errorf("node is not a ProtoNode")
	}
	nodeBytes, err := protoNode.EncodeProtobuf(false)
	if err != nil {
		return fmt.Errorf("encode node to protobuf: %w", err)
	}

	if err = d.store.Put(ctx, d.nodeKey(node.Cid()), nodeBytes); err != nil {
		return fmt.Errorf("put node %s in local store: %w", node, err)
	}
	slog.Debug("Persisted node in local store", "cid", node.Cid(), "size", len(nodeBytes))

	// TODO: Think about broadcasting the new node to the peers to not require each peer to query the node
	//  when a new CID is broadcasted.

	return nil
}

func (d *dagSyncer) AddMany(ctx context.Context, nodes []ipld.Node) error {
	panic("implement me")
}

func (d *dagSyncer) Remove(ctx context.Context, cid cid.Cid) error {
	panic("implement me")
}

func (d *dagSyncer) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	panic("implement me")
}

func (d *dagSyncer) nodeKey(cid cid.Cid) ds.Key {
	return d.namespace.Child(dshelp.MultihashToDsKey(cid.Hash()))
}

var _ ipld.DAGService = (*dagSyncer)(nil)
