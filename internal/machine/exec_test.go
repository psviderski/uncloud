package machine

import (
	"context"
	"io"
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
	requests  []*pb.ExecCommandRequest
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

func (s *execCommandStream) Recv() (*pb.ExecCommandRequest, error) {
	if len(s.requests) == 0 {
		return nil, io.EOF
	}
	req := s.requests[0]
	s.requests = s.requests[1:]
	return req, nil
}

func TestExecCommand(t *testing.T) {
	stream := &execCommandStream{
		ctx: context.Background(),
		requests: []*pb.ExecCommandRequest{
			{
				Payload: &pb.ExecCommandRequest_Config{
					Config: &pb.ExecCommandConfig{
						Command: []string{"sh", "-c", "printf stdout; printf stderr >&2; exit 7"},
					},
				},
			},
		},
	}

	err := (&Machine{}).ExecCommand(stream)
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

func TestExecCommandWithStdin(t *testing.T) {
	stream := &execCommandStream{
		ctx: context.Background(),
		requests: []*pb.ExecCommandRequest{
			{
				Payload: &pb.ExecCommandRequest_Config{
					Config: &pb.ExecCommandConfig{
						Command:     []string{"cat"},
						AttachStdin: true,
					},
				},
			},
			{
				Payload: &pb.ExecCommandRequest_Stdin{Stdin: []byte("hello")},
			},
		},
	}

	err := (&Machine{}).ExecCommand(stream)
	require.NoError(t, err)

	var stdout []byte
	exitCode := -1
	for _, resp := range stream.responses {
		switch payload := resp.Payload.(type) {
		case *pb.ExecCommandResponse_Stdout:
			stdout = append(stdout, payload.Stdout...)
		case *pb.ExecCommandResponse_ExitCode:
			exitCode = int(payload.ExitCode)
		}
	}

	assert.Equal(t, "hello", string(stdout))
	assert.Equal(t, 0, exitCode)
}

func TestExecCommandRequiresCommand(t *testing.T) {
	stream := &execCommandStream{
		ctx: context.Background(),
		requests: []*pb.ExecCommandRequest{
			{
				Payload: &pb.ExecCommandRequest_Config{
					Config: &pb.ExecCommandConfig{},
				},
			},
		},
	}

	err := (&Machine{}).ExecCommand(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
