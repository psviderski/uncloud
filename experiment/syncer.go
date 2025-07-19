package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hashicorp/serf/serf"
	"github.com/ipfs/boxo/datastore/dshelp"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ipld "github.com/ipfs/go-ipld-format"
)

// Implements the DAGService interface.
// TODO: implement SessionDAGService to optimize node fetching.
// TODO: persistentSerfDAG?
type dagSyncer struct {
	// Persistent storage for the nodes.
	store     ds.Datastore
	namespace ds.Key
	serf      *serf.Serf
}

func newDAGSyncer(store ds.Datastore, namespace ds.Key, serf *serf.Serf) *dagSyncer {
	return &dagSyncer{
		store:     store,
		namespace: namespace,
		serf:      serf,
	}
}

func (d *dagSyncer) Get(ctx context.Context, cid cid.Cid) (ipld.Node, error) {
	slog.Debug("Getting node", "cid", cid)
	node, err := d.getNode(ctx, cid)
	if err == nil {
		slog.Debug("Found node in local store", "cid", cid)
		return node, nil
	}
	if !errors.Is(err, ds.ErrNotFound) {
		return nil, err
	}

	// The node it not found in the local store. Try to retrieve it from remote peers.
	// TODO: exclude the local peer from the query.
	query, err := d.serf.Query("get-node", cid.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("query node from peers: %w", err)
	}
	slog.Debug("Queried node from peers", "cid", cid, "deadline", query.Deadline())

	for {
		select {
		case resp, ok := <-query.ResponseCh():
			if !ok {
				// The query has finished and no response was received.
				slog.Warn("Query for node finished without response", "cid", cid)
				return nil, ipld.ErrNotFound{Cid: cid}
			}
			if resp.From == d.serf.LocalMember().Name {
				continue
			}
			slog.Debug("Received node from peer", "cid", cid, "peer", resp.From)
			query.Close()

			node, err = nodeFromBytes(resp.Payload)
			if err != nil {
				return nil, err
			}
			// Ensure the received node is actually the requested node.
			if node.Cid() != cid {
				return nil, fmt.Errorf("received node CID %s does not match requested CID %s", node.Cid(), cid)
			}
			if err = d.persistNode(ctx, node); err != nil {
				return nil, err
			}
			return node, nil
		case <-ctx.Done():
			query.Close()
			return nil, ctx.Err()
		}
	}
	// This return should never be reached.
	return nil, ipld.ErrNotFound{Cid: cid}
}

func (d *dagSyncer) getNode(ctx context.Context, cid cid.Cid) (*dag.ProtoNode, error) {
	bytes, err := d.store.Get(ctx, d.nodeKey(cid))
	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return nil, ds.ErrNotFound
		}
		return nil, fmt.Errorf("get node %s from local store: %w", cid, err)
	}
	return nodeFromBytes(bytes)
}

func (d *dagSyncer) persistNode(ctx context.Context, node *dag.ProtoNode) error {
	bytes, err := node.EncodeProtobuf(false)
	if err != nil {
		return fmt.Errorf("encode node to protobuf: %w", err)
	}
	if err = d.store.Put(ctx, d.nodeKey(node.Cid()), bytes); err != nil {
		return fmt.Errorf("put node %s in local store: %w", node.String(), err)
	}
	slog.Debug("Persisted node in local store", "cid", node.Cid(), "size", len(bytes))
	return nil
}

func (d *dagSyncer) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	ch := make(chan *ipld.NodeOption)
	go func() {
		defer close(ch)
		for _, cid := range cids {
			node, err := d.Get(ctx, cid)
			if err != nil {
				ch <- &ipld.NodeOption{Err: err}
			} else {
				ch <- &ipld.NodeOption{Node: node}
			}
		}
	}()
	return ch
}

func (d *dagSyncer) Add(ctx context.Context, node ipld.Node) error {
	slog.Debug("Adding node", "cid", node.Cid())
	protoNode, ok := node.(*dag.ProtoNode)
	if !ok {
		return fmt.Errorf("node is not a ProtoNode")
	}
	if err := d.persistNode(ctx, protoNode); err != nil {
		return err
	}
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

// TODO: perhaps this should not be part of the syncer/DAG service?
func (d *dagSyncer) HandleEvent(event serf.Event) {
	query, ok := event.(*serf.Query)
	if !ok || query.Name != "get-node" {
		// Ignore non-get-node queries.
		return
	}
	_, cid, err := cid.CidFromBytes(query.Payload)
	if err != nil {
		slog.Error("Decode CID from query payload", "error", err)
		return
	}
	slog.Debug("Received get-node query", "cid", cid)
	log := slog.With("cid", cid)

	node, err := d.getNode(context.Background(), cid)
	if err != nil {
		if !errors.Is(err, ds.ErrNotFound) {
			log.Error("Get node from local store", "error", err)
			return
		}
		// The node is not found in the local store. Ignore the query.
		log.Debug("Node not found in local store")
		return
	}
	bytes, err := node.EncodeProtobuf(false)
	if err != nil {
		log.Error("Encode node to protobuf", "error", err)
	}
	if err = query.Respond(bytes); err != nil {
		log.Error("Respond to get-node query", "error", err)
		return
	}
	log.Debug("Responded to get-node query")
}

func (d *dagSyncer) nodeKey(cid cid.Cid) ds.Key {
	return d.namespace.Child(dshelp.MultihashToDsKey(cid.Hash()))
}

var _ ipld.DAGService = (*dagSyncer)(nil)

func nodeFromBytes(nodeBytes []byte) (*dag.ProtoNode, error) {
	node, err := dag.DecodeProtobuf(nodeBytes)
	if err != nil {
		return nil, fmt.Errorf("decode node from protobuf: %w", err)
	}
	// CID is lazily computed from the node content. Ensure the node uses CIDv1.
	if err = node.SetCidBuilder(dag.V1CidPrefix()); err != nil {
		return nil, fmt.Errorf("set CIDv1 on node: %w", err)
	}
	return node, nil
}
