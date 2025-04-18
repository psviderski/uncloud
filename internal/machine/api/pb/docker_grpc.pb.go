// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.27.3
// source: internal/machine/api/pb/docker.proto

package pb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Docker_CreateContainer_FullMethodName         = "/api.Docker/CreateContainer"
	Docker_InspectContainer_FullMethodName        = "/api.Docker/InspectContainer"
	Docker_StartContainer_FullMethodName          = "/api.Docker/StartContainer"
	Docker_StopContainer_FullMethodName           = "/api.Docker/StopContainer"
	Docker_ListContainers_FullMethodName          = "/api.Docker/ListContainers"
	Docker_RemoveContainer_FullMethodName         = "/api.Docker/RemoveContainer"
	Docker_PullImage_FullMethodName               = "/api.Docker/PullImage"
	Docker_InspectImage_FullMethodName            = "/api.Docker/InspectImage"
	Docker_InspectRemoteImage_FullMethodName      = "/api.Docker/InspectRemoteImage"
	Docker_CreateVolume_FullMethodName            = "/api.Docker/CreateVolume"
	Docker_ListVolumes_FullMethodName             = "/api.Docker/ListVolumes"
	Docker_RemoveVolume_FullMethodName            = "/api.Docker/RemoveVolume"
	Docker_CreateServiceContainer_FullMethodName  = "/api.Docker/CreateServiceContainer"
	Docker_InspectServiceContainer_FullMethodName = "/api.Docker/InspectServiceContainer"
	Docker_ListServiceContainers_FullMethodName   = "/api.Docker/ListServiceContainers"
	Docker_RemoveServiceContainer_FullMethodName  = "/api.Docker/RemoveServiceContainer"
)

// DockerClient is the client API for Docker service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type DockerClient interface {
	CreateContainer(ctx context.Context, in *CreateContainerRequest, opts ...grpc.CallOption) (*CreateContainerResponse, error)
	InspectContainer(ctx context.Context, in *InspectContainerRequest, opts ...grpc.CallOption) (*InspectContainerResponse, error)
	StartContainer(ctx context.Context, in *StartContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	StopContainer(ctx context.Context, in *StopContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ListContainers(ctx context.Context, in *ListContainersRequest, opts ...grpc.CallOption) (*ListContainersResponse, error)
	RemoveContainer(ctx context.Context, in *RemoveContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	PullImage(ctx context.Context, in *PullImageRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[JSONMessage], error)
	InspectImage(ctx context.Context, in *InspectImageRequest, opts ...grpc.CallOption) (*InspectImageResponse, error)
	// InspectRemoteImage returns the image metadata for an image in a remote registry using the machine's
	// Docker auth credentials if necessary.
	InspectRemoteImage(ctx context.Context, in *InspectRemoteImageRequest, opts ...grpc.CallOption) (*InspectRemoteImageResponse, error)
	CreateVolume(ctx context.Context, in *CreateVolumeRequest, opts ...grpc.CallOption) (*CreateVolumeResponse, error)
	ListVolumes(ctx context.Context, in *ListVolumesRequest, opts ...grpc.CallOption) (*ListVolumesResponse, error)
	RemoveVolume(ctx context.Context, in *RemoveVolumeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	CreateServiceContainer(ctx context.Context, in *CreateServiceContainerRequest, opts ...grpc.CallOption) (*CreateContainerResponse, error)
	InspectServiceContainer(ctx context.Context, in *InspectContainerRequest, opts ...grpc.CallOption) (*ServiceContainer, error)
	ListServiceContainers(ctx context.Context, in *ListServiceContainersRequest, opts ...grpc.CallOption) (*ListServiceContainersResponse, error)
	RemoveServiceContainer(ctx context.Context, in *RemoveContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type dockerClient struct {
	cc grpc.ClientConnInterface
}

func NewDockerClient(cc grpc.ClientConnInterface) DockerClient {
	return &dockerClient{cc}
}

func (c *dockerClient) CreateContainer(ctx context.Context, in *CreateContainerRequest, opts ...grpc.CallOption) (*CreateContainerResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateContainerResponse)
	err := c.cc.Invoke(ctx, Docker_CreateContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) InspectContainer(ctx context.Context, in *InspectContainerRequest, opts ...grpc.CallOption) (*InspectContainerResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InspectContainerResponse)
	err := c.cc.Invoke(ctx, Docker_InspectContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) StartContainer(ctx context.Context, in *StartContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Docker_StartContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) StopContainer(ctx context.Context, in *StopContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Docker_StopContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) ListContainers(ctx context.Context, in *ListContainersRequest, opts ...grpc.CallOption) (*ListContainersResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListContainersResponse)
	err := c.cc.Invoke(ctx, Docker_ListContainers_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) RemoveContainer(ctx context.Context, in *RemoveContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Docker_RemoveContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) PullImage(ctx context.Context, in *PullImageRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[JSONMessage], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Docker_ServiceDesc.Streams[0], Docker_PullImage_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[PullImageRequest, JSONMessage]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Docker_PullImageClient = grpc.ServerStreamingClient[JSONMessage]

func (c *dockerClient) InspectImage(ctx context.Context, in *InspectImageRequest, opts ...grpc.CallOption) (*InspectImageResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InspectImageResponse)
	err := c.cc.Invoke(ctx, Docker_InspectImage_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) InspectRemoteImage(ctx context.Context, in *InspectRemoteImageRequest, opts ...grpc.CallOption) (*InspectRemoteImageResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InspectRemoteImageResponse)
	err := c.cc.Invoke(ctx, Docker_InspectRemoteImage_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) CreateVolume(ctx context.Context, in *CreateVolumeRequest, opts ...grpc.CallOption) (*CreateVolumeResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateVolumeResponse)
	err := c.cc.Invoke(ctx, Docker_CreateVolume_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) ListVolumes(ctx context.Context, in *ListVolumesRequest, opts ...grpc.CallOption) (*ListVolumesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListVolumesResponse)
	err := c.cc.Invoke(ctx, Docker_ListVolumes_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) RemoveVolume(ctx context.Context, in *RemoveVolumeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Docker_RemoveVolume_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) CreateServiceContainer(ctx context.Context, in *CreateServiceContainerRequest, opts ...grpc.CallOption) (*CreateContainerResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateContainerResponse)
	err := c.cc.Invoke(ctx, Docker_CreateServiceContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) InspectServiceContainer(ctx context.Context, in *InspectContainerRequest, opts ...grpc.CallOption) (*ServiceContainer, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ServiceContainer)
	err := c.cc.Invoke(ctx, Docker_InspectServiceContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) ListServiceContainers(ctx context.Context, in *ListServiceContainersRequest, opts ...grpc.CallOption) (*ListServiceContainersResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListServiceContainersResponse)
	err := c.cc.Invoke(ctx, Docker_ListServiceContainers_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) RemoveServiceContainer(ctx context.Context, in *RemoveContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Docker_RemoveServiceContainer_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DockerServer is the server API for Docker service.
// All implementations must embed UnimplementedDockerServer
// for forward compatibility.
type DockerServer interface {
	CreateContainer(context.Context, *CreateContainerRequest) (*CreateContainerResponse, error)
	InspectContainer(context.Context, *InspectContainerRequest) (*InspectContainerResponse, error)
	StartContainer(context.Context, *StartContainerRequest) (*emptypb.Empty, error)
	StopContainer(context.Context, *StopContainerRequest) (*emptypb.Empty, error)
	ListContainers(context.Context, *ListContainersRequest) (*ListContainersResponse, error)
	RemoveContainer(context.Context, *RemoveContainerRequest) (*emptypb.Empty, error)
	PullImage(*PullImageRequest, grpc.ServerStreamingServer[JSONMessage]) error
	InspectImage(context.Context, *InspectImageRequest) (*InspectImageResponse, error)
	// InspectRemoteImage returns the image metadata for an image in a remote registry using the machine's
	// Docker auth credentials if necessary.
	InspectRemoteImage(context.Context, *InspectRemoteImageRequest) (*InspectRemoteImageResponse, error)
	CreateVolume(context.Context, *CreateVolumeRequest) (*CreateVolumeResponse, error)
	ListVolumes(context.Context, *ListVolumesRequest) (*ListVolumesResponse, error)
	RemoveVolume(context.Context, *RemoveVolumeRequest) (*emptypb.Empty, error)
	CreateServiceContainer(context.Context, *CreateServiceContainerRequest) (*CreateContainerResponse, error)
	InspectServiceContainer(context.Context, *InspectContainerRequest) (*ServiceContainer, error)
	ListServiceContainers(context.Context, *ListServiceContainersRequest) (*ListServiceContainersResponse, error)
	RemoveServiceContainer(context.Context, *RemoveContainerRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedDockerServer()
}

// UnimplementedDockerServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedDockerServer struct{}

func (UnimplementedDockerServer) CreateContainer(context.Context, *CreateContainerRequest) (*CreateContainerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateContainer not implemented")
}
func (UnimplementedDockerServer) InspectContainer(context.Context, *InspectContainerRequest) (*InspectContainerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InspectContainer not implemented")
}
func (UnimplementedDockerServer) StartContainer(context.Context, *StartContainerRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartContainer not implemented")
}
func (UnimplementedDockerServer) StopContainer(context.Context, *StopContainerRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StopContainer not implemented")
}
func (UnimplementedDockerServer) ListContainers(context.Context, *ListContainersRequest) (*ListContainersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListContainers not implemented")
}
func (UnimplementedDockerServer) RemoveContainer(context.Context, *RemoveContainerRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RemoveContainer not implemented")
}
func (UnimplementedDockerServer) PullImage(*PullImageRequest, grpc.ServerStreamingServer[JSONMessage]) error {
	return status.Errorf(codes.Unimplemented, "method PullImage not implemented")
}
func (UnimplementedDockerServer) InspectImage(context.Context, *InspectImageRequest) (*InspectImageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InspectImage not implemented")
}
func (UnimplementedDockerServer) InspectRemoteImage(context.Context, *InspectRemoteImageRequest) (*InspectRemoteImageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InspectRemoteImage not implemented")
}
func (UnimplementedDockerServer) CreateVolume(context.Context, *CreateVolumeRequest) (*CreateVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateVolume not implemented")
}
func (UnimplementedDockerServer) ListVolumes(context.Context, *ListVolumesRequest) (*ListVolumesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListVolumes not implemented")
}
func (UnimplementedDockerServer) RemoveVolume(context.Context, *RemoveVolumeRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RemoveVolume not implemented")
}
func (UnimplementedDockerServer) CreateServiceContainer(context.Context, *CreateServiceContainerRequest) (*CreateContainerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateServiceContainer not implemented")
}
func (UnimplementedDockerServer) InspectServiceContainer(context.Context, *InspectContainerRequest) (*ServiceContainer, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InspectServiceContainer not implemented")
}
func (UnimplementedDockerServer) ListServiceContainers(context.Context, *ListServiceContainersRequest) (*ListServiceContainersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListServiceContainers not implemented")
}
func (UnimplementedDockerServer) RemoveServiceContainer(context.Context, *RemoveContainerRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RemoveServiceContainer not implemented")
}
func (UnimplementedDockerServer) mustEmbedUnimplementedDockerServer() {}
func (UnimplementedDockerServer) testEmbeddedByValue()                {}

// UnsafeDockerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to DockerServer will
// result in compilation errors.
type UnsafeDockerServer interface {
	mustEmbedUnimplementedDockerServer()
}

func RegisterDockerServer(s grpc.ServiceRegistrar, srv DockerServer) {
	// If the following call pancis, it indicates UnimplementedDockerServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Docker_ServiceDesc, srv)
}

func _Docker_CreateContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).CreateContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_CreateContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).CreateContainer(ctx, req.(*CreateContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_InspectContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).InspectContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_InspectContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).InspectContainer(ctx, req.(*InspectContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_StartContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).StartContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_StartContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).StartContainer(ctx, req.(*StartContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_StopContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).StopContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_StopContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).StopContainer(ctx, req.(*StopContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_ListContainers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListContainersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).ListContainers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_ListContainers_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).ListContainers(ctx, req.(*ListContainersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_RemoveContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RemoveContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).RemoveContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_RemoveContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).RemoveContainer(ctx, req.(*RemoveContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_PullImage_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(PullImageRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(DockerServer).PullImage(m, &grpc.GenericServerStream[PullImageRequest, JSONMessage]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Docker_PullImageServer = grpc.ServerStreamingServer[JSONMessage]

func _Docker_InspectImage_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectImageRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).InspectImage(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_InspectImage_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).InspectImage(ctx, req.(*InspectImageRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_InspectRemoteImage_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectRemoteImageRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).InspectRemoteImage(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_InspectRemoteImage_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).InspectRemoteImage(ctx, req.(*InspectRemoteImageRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_CreateVolume_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateVolumeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).CreateVolume(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_CreateVolume_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).CreateVolume(ctx, req.(*CreateVolumeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_ListVolumes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListVolumesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).ListVolumes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_ListVolumes_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).ListVolumes(ctx, req.(*ListVolumesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_RemoveVolume_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RemoveVolumeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).RemoveVolume(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_RemoveVolume_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).RemoveVolume(ctx, req.(*RemoveVolumeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_CreateServiceContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateServiceContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).CreateServiceContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_CreateServiceContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).CreateServiceContainer(ctx, req.(*CreateServiceContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_InspectServiceContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).InspectServiceContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_InspectServiceContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).InspectServiceContainer(ctx, req.(*InspectContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_ListServiceContainers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListServiceContainersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).ListServiceContainers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_ListServiceContainers_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).ListServiceContainers(ctx, req.(*ListServiceContainersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Docker_RemoveServiceContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RemoveContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DockerServer).RemoveServiceContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Docker_RemoveServiceContainer_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DockerServer).RemoveServiceContainer(ctx, req.(*RemoveContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Docker_ServiceDesc is the grpc.ServiceDesc for Docker service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Docker_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "api.Docker",
	HandlerType: (*DockerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateContainer",
			Handler:    _Docker_CreateContainer_Handler,
		},
		{
			MethodName: "InspectContainer",
			Handler:    _Docker_InspectContainer_Handler,
		},
		{
			MethodName: "StartContainer",
			Handler:    _Docker_StartContainer_Handler,
		},
		{
			MethodName: "StopContainer",
			Handler:    _Docker_StopContainer_Handler,
		},
		{
			MethodName: "ListContainers",
			Handler:    _Docker_ListContainers_Handler,
		},
		{
			MethodName: "RemoveContainer",
			Handler:    _Docker_RemoveContainer_Handler,
		},
		{
			MethodName: "InspectImage",
			Handler:    _Docker_InspectImage_Handler,
		},
		{
			MethodName: "InspectRemoteImage",
			Handler:    _Docker_InspectRemoteImage_Handler,
		},
		{
			MethodName: "CreateVolume",
			Handler:    _Docker_CreateVolume_Handler,
		},
		{
			MethodName: "ListVolumes",
			Handler:    _Docker_ListVolumes_Handler,
		},
		{
			MethodName: "RemoveVolume",
			Handler:    _Docker_RemoveVolume_Handler,
		},
		{
			MethodName: "CreateServiceContainer",
			Handler:    _Docker_CreateServiceContainer_Handler,
		},
		{
			MethodName: "InspectServiceContainer",
			Handler:    _Docker_InspectServiceContainer_Handler,
		},
		{
			MethodName: "ListServiceContainers",
			Handler:    _Docker_ListServiceContainers_Handler,
		},
		{
			MethodName: "RemoveServiceContainer",
			Handler:    _Docker_RemoveServiceContainer_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "PullImage",
			Handler:       _Docker_PullImage_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "internal/machine/api/pb/docker.proto",
}
