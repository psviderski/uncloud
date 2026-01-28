package proxy

import (
	"fmt"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/siderolabs/grpc-proxy/proxy"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// MetadataBackend wraps a proxy.Backend and injects machine metadata into responses in One2Many mode.
type MetadataBackend struct {
	proxy.Backend
	MachineID   string
	MachineName string
	MachineAddr string
}

// AppendInfo is called to enhance response from the backend with additional data.
func (b *MetadataBackend) AppendInfo(streaming bool, resp []byte) ([]byte, error) {
	payload, err := proto.Marshal(&pb.Empty{
		Metadata: &pb.Metadata{
			Machine:     b.MachineAddr,
			MachineId:   b.MachineID,
			MachineName: b.MachineName,
		},
	})

	if streaming {
		return append(resp, payload...), err
	}

	const (
		metadataField = 1 // field number in proto definition for repeated response
		metadataType  = 2 // "string" for embedded messages
	)

	// decode protobuf embedded header

	typ, n1 := protowire.ConsumeVarint(resp)
	if n1 < 0 {
		return nil, protowire.ParseError(n1)
	}

	_, n2 := protowire.ConsumeVarint(resp[n1:]) // length
	if n2 < 0 {
		return nil, protowire.ParseError(n2)
	}

	if typ != (metadataField<<3)|metadataType {
		return nil, fmt.Errorf("unexpected message format: %d", typ)
	}

	if n1+n2 > len(resp) {
		return nil, fmt.Errorf("unexpected message size: %d", len(resp))
	}

	// cut off embedded message header
	resp = resp[n1+n2:]
	// build new embedded message header
	prefix := protowire.AppendVarint(
		protowire.AppendVarint(nil, (metadataField<<3)|metadataType),
		uint64(len(resp)+len(payload)),
	)
	resp = append(prefix, resp...)

	return append(resp, payload...), err
}

// BuildError converts upstream error into message from upstream, so that multiple
// successful and failure responses might be returned.
func (b *MetadataBackend) BuildError(streaming bool, err error) ([]byte, error) {
	var resp proto.Message = &pb.Empty{
		Metadata: &pb.Metadata{
			Machine:     b.MachineAddr,
			MachineId:   b.MachineID,
			MachineName: b.MachineName,
			Error:       err.Error(),
			Status:      status.Convert(err).Proto(),
		},
	}

	if !streaming {
		resp = &pb.EmptyResponse{
			Messages: []*pb.Empty{
				resp.(*pb.Empty),
			},
		}
	}

	return proto.Marshal(resp)
}
