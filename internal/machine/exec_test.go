package machine

import (
	"context"
	"testing"

	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type execCommandStream struct {
	grpc.ServerStream
	ctx       context.Context
	responses []*pb.ExecCommandResponse
}

func (s *execCommandStream) Context() context.Context {
	return s.ctx
}

func (s *execCommandStream) Send(resp *pb.ExecCommandResponse) error {
	s.responses = append(s.responses, resp)
	return nil
}

func TestExecCommand(t *testing.T) {
	stream := &execCommandStream{ctx: context.Background()}

	err := (&Machine{}).ExecCommand(&pb.ExecCommandRequest{
		Command: []string{"sh", "-c", "printf stdout; printf stderr >&2; exit 7"},
	}, stream)
	require.NoError(t, err)

	var stdout, stderr []byte
	exitCode := -1
	for _, resp := range stream.responses {
		switch payload := resp.Payload.(type) {
		case *pb.ExecCommandResponse_Stdout:
			stdout = append(stdout, payload.Stdout...)
		case *pb.ExecCommandResponse_Stderr:
			stderr = append(stderr, payload.Stderr...)
		case *pb.ExecCommandResponse_ExitCode:
			exitCode = int(payload.ExitCode)
		}
	}

	assert.Equal(t, "stdout", string(stdout))
	assert.Equal(t, "stderr", string(stderr))
	assert.Equal(t, 7, exitCode)
}

func TestExecCommandRequiresCommand(t *testing.T) {
	stream := &execCommandStream{ctx: context.Background()}

	err := (&Machine{}).ExecCommand(&pb.ExecCommandRequest{}, stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
