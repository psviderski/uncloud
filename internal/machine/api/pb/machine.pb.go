// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.27.3
// source: internal/machine/api/pb/machine.proto

package pb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type MachineInfo struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id      string         `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name    string         `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Network *NetworkConfig `protobuf:"bytes,3,opt,name=network,proto3" json:"network,omitempty"`
}

func (x *MachineInfo) Reset() {
	*x = MachineInfo{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MachineInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MachineInfo) ProtoMessage() {}

func (x *MachineInfo) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MachineInfo.ProtoReflect.Descriptor instead.
func (*MachineInfo) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{0}
}

func (x *MachineInfo) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *MachineInfo) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *MachineInfo) GetNetwork() *NetworkConfig {
	if x != nil {
		return x.Network
	}
	return nil
}

type NetworkConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Subnet       *IPPrefix `protobuf:"bytes,1,opt,name=subnet,proto3" json:"subnet,omitempty"`
	ManagementIp *IP       `protobuf:"bytes,2,opt,name=management_ip,json=managementIp,proto3" json:"management_ip,omitempty"`
	Endpoints    []*IPPort `protobuf:"bytes,3,rep,name=endpoints,proto3" json:"endpoints,omitempty"`
	PublicKey    []byte    `protobuf:"bytes,4,opt,name=publicKey,proto3" json:"publicKey,omitempty"`
}

func (x *NetworkConfig) Reset() {
	*x = NetworkConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *NetworkConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NetworkConfig) ProtoMessage() {}

func (x *NetworkConfig) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use NetworkConfig.ProtoReflect.Descriptor instead.
func (*NetworkConfig) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{1}
}

func (x *NetworkConfig) GetSubnet() *IPPrefix {
	if x != nil {
		return x.Subnet
	}
	return nil
}

func (x *NetworkConfig) GetManagementIp() *IP {
	if x != nil {
		return x.ManagementIp
	}
	return nil
}

func (x *NetworkConfig) GetEndpoints() []*IPPort {
	if x != nil {
		return x.Endpoints
	}
	return nil
}

func (x *NetworkConfig) GetPublicKey() []byte {
	if x != nil {
		return x.PublicKey
	}
	return nil
}

type InitClusterRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	MachineName string    `protobuf:"bytes,1,opt,name=machineName,proto3" json:"machineName,omitempty"`
	Network     *IPPrefix `protobuf:"bytes,2,opt,name=network,proto3" json:"network,omitempty"`
}

func (x *InitClusterRequest) Reset() {
	*x = InitClusterRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InitClusterRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InitClusterRequest) ProtoMessage() {}

func (x *InitClusterRequest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InitClusterRequest.ProtoReflect.Descriptor instead.
func (*InitClusterRequest) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{2}
}

func (x *InitClusterRequest) GetMachineName() string {
	if x != nil {
		return x.MachineName
	}
	return ""
}

func (x *InitClusterRequest) GetNetwork() *IPPrefix {
	if x != nil {
		return x.Network
	}
	return nil
}

type InitClusterResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Machine *MachineInfo `protobuf:"bytes,1,opt,name=machine,proto3" json:"machine,omitempty"`
}

func (x *InitClusterResponse) Reset() {
	*x = InitClusterResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InitClusterResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InitClusterResponse) ProtoMessage() {}

func (x *InitClusterResponse) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InitClusterResponse.ProtoReflect.Descriptor instead.
func (*InitClusterResponse) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{3}
}

func (x *InitClusterResponse) GetMachine() *MachineInfo {
	if x != nil {
		return x.Machine
	}
	return nil
}

type JoinClusterRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Machine       *MachineInfo   `protobuf:"bytes,1,opt,name=machine,proto3" json:"machine,omitempty"`
	OtherMachines []*MachineInfo `protobuf:"bytes,3,rep,name=other_machines,json=otherMachines,proto3" json:"other_machines,omitempty"`
}

func (x *JoinClusterRequest) Reset() {
	*x = JoinClusterRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *JoinClusterRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*JoinClusterRequest) ProtoMessage() {}

func (x *JoinClusterRequest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use JoinClusterRequest.ProtoReflect.Descriptor instead.
func (*JoinClusterRequest) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{4}
}

func (x *JoinClusterRequest) GetMachine() *MachineInfo {
	if x != nil {
		return x.Machine
	}
	return nil
}

func (x *JoinClusterRequest) GetOtherMachines() []*MachineInfo {
	if x != nil {
		return x.OtherMachines
	}
	return nil
}

type TokenResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Token string `protobuf:"bytes,1,opt,name=token,proto3" json:"token,omitempty"`
}

func (x *TokenResponse) Reset() {
	*x = TokenResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TokenResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TokenResponse) ProtoMessage() {}

func (x *TokenResponse) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TokenResponse.ProtoReflect.Descriptor instead.
func (*TokenResponse) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{5}
}

func (x *TokenResponse) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

type Service struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id         string               `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name       string               `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Mode       string               `protobuf:"bytes,3,opt,name=mode,proto3" json:"mode,omitempty"`
	Containers []*Service_Container `protobuf:"bytes,4,rep,name=containers,proto3" json:"containers,omitempty"`
}

func (x *Service) Reset() {
	*x = Service{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Service) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Service) ProtoMessage() {}

func (x *Service) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Service.ProtoReflect.Descriptor instead.
func (*Service) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{6}
}

func (x *Service) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *Service) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Service) GetMode() string {
	if x != nil {
		return x.Mode
	}
	return ""
}

func (x *Service) GetContainers() []*Service_Container {
	if x != nil {
		return x.Containers
	}
	return nil
}

type InspectServiceRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *InspectServiceRequest) Reset() {
	*x = InspectServiceRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InspectServiceRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InspectServiceRequest) ProtoMessage() {}

func (x *InspectServiceRequest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InspectServiceRequest.ProtoReflect.Descriptor instead.
func (*InspectServiceRequest) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{7}
}

func (x *InspectServiceRequest) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

type InspectServiceResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Service *Service `protobuf:"bytes,1,opt,name=service,proto3" json:"service,omitempty"`
}

func (x *InspectServiceResponse) Reset() {
	*x = InspectServiceResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *InspectServiceResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InspectServiceResponse) ProtoMessage() {}

func (x *InspectServiceResponse) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InspectServiceResponse.ProtoReflect.Descriptor instead.
func (*InspectServiceResponse) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{8}
}

func (x *InspectServiceResponse) GetService() *Service {
	if x != nil {
		return x.Service
	}
	return nil
}

type Service_Container struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	MachineId string `protobuf:"bytes,1,opt,name=machine_id,json=machineId,proto3" json:"machine_id,omitempty"`
	// JSON encoded Docker types.Container.
	Container []byte `protobuf:"bytes,2,opt,name=container,proto3" json:"container,omitempty"`
}

func (x *Service_Container) Reset() {
	*x = Service_Container{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_machine_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Service_Container) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Service_Container) ProtoMessage() {}

func (x *Service_Container) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_machine_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Service_Container.ProtoReflect.Descriptor instead.
func (*Service_Container) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_machine_proto_rawDescGZIP(), []int{6, 0}
}

func (x *Service_Container) GetMachineId() string {
	if x != nil {
		return x.MachineId
	}
	return ""
}

func (x *Service_Container) GetContainer() []byte {
	if x != nil {
		return x.Container
	}
	return nil
}

var File_internal_machine_api_pb_machine_proto protoreflect.FileDescriptor

var file_internal_machine_api_pb_machine_proto_rawDesc = []byte{
	0x0a, 0x25, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61, 0x63, 0x68, 0x69,
	0x6e, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x62, 0x2f, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03, 0x61, 0x70, 0x69, 0x1a, 0x1b, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d,
	0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x24, 0x69, 0x6e, 0x74, 0x65, 0x72,
	0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x70, 0x62, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x5f, 0x0a, 0x0b, 0x4d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x12,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x12, 0x2c, 0x0a, 0x07, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x4e, 0x65, 0x74, 0x77, 0x6f, 0x72,
	0x6b, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x07, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b,
	0x22, 0xad, 0x01, 0x0a, 0x0d, 0x4e, 0x65, 0x74, 0x77, 0x6f, 0x72, 0x6b, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x12, 0x25, 0x0a, 0x06, 0x73, 0x75, 0x62, 0x6e, 0x65, 0x74, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x0d, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x49, 0x50, 0x50, 0x72, 0x65, 0x66, 0x69,
	0x78, 0x52, 0x06, 0x73, 0x75, 0x62, 0x6e, 0x65, 0x74, 0x12, 0x2c, 0x0a, 0x0d, 0x6d, 0x61, 0x6e,
	0x61, 0x67, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x5f, 0x69, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x07, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x49, 0x50, 0x52, 0x0c, 0x6d, 0x61, 0x6e, 0x61, 0x67,
	0x65, 0x6d, 0x65, 0x6e, 0x74, 0x49, 0x70, 0x12, 0x29, 0x0a, 0x09, 0x65, 0x6e, 0x64, 0x70, 0x6f,
	0x69, 0x6e, 0x74, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x0b, 0x2e, 0x61, 0x70, 0x69,
	0x2e, 0x49, 0x50, 0x50, 0x6f, 0x72, 0x74, 0x52, 0x09, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e,
	0x74, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79,
	0x22, 0x5f, 0x0a, 0x12, 0x49, 0x6e, 0x69, 0x74, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x20, 0x0a, 0x0b, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e,
	0x65, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x6d, 0x61, 0x63,
	0x68, 0x69, 0x6e, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x27, 0x0a, 0x07, 0x6e, 0x65, 0x74, 0x77,
	0x6f, 0x72, 0x6b, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0d, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x49, 0x50, 0x50, 0x72, 0x65, 0x66, 0x69, 0x78, 0x52, 0x07, 0x6e, 0x65, 0x74, 0x77, 0x6f, 0x72,
	0x6b, 0x22, 0x41, 0x0a, 0x13, 0x49, 0x6e, 0x69, 0x74, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x2a, 0x0a, 0x07, 0x6d, 0x61, 0x63, 0x68,
	0x69, 0x6e, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x4d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x07, 0x6d, 0x61, 0x63,
	0x68, 0x69, 0x6e, 0x65, 0x22, 0x79, 0x0a, 0x12, 0x4a, 0x6f, 0x69, 0x6e, 0x43, 0x6c, 0x75, 0x73,
	0x74, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2a, 0x0a, 0x07, 0x6d, 0x61,
	0x63, 0x68, 0x69, 0x6e, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x61, 0x70,
	0x69, 0x2e, 0x4d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x07, 0x6d,
	0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x12, 0x37, 0x0a, 0x0e, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x5f,
	0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x10,
	0x2e, 0x61, 0x70, 0x69, 0x2e, 0x4d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x49, 0x6e, 0x66, 0x6f,
	0x52, 0x0d, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x4d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x73, 0x22,
	0x25, 0x0a, 0x0d, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x14, 0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0xc3, 0x01, 0x0a, 0x07, 0x53, 0x65, 0x72, 0x76, 0x69,
	0x63, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02,
	0x69, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x12, 0x36, 0x0a, 0x0a, 0x63, 0x6f,
	0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x73, 0x18, 0x04, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x16,
	0x2e, 0x61, 0x70, 0x69, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x43, 0x6f, 0x6e,
	0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x52, 0x0a, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65,
	0x72, 0x73, 0x1a, 0x48, 0x0a, 0x09, 0x43, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x12,
	0x1d, 0x0a, 0x0a, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x09, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x49, 0x64, 0x12, 0x1c,
	0x0a, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x22, 0x27, 0x0a, 0x15,
	0x49, 0x6e, 0x73, 0x70, 0x65, 0x63, 0x74, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x02, 0x69, 0x64, 0x22, 0x40, 0x0a, 0x16, 0x49, 0x6e, 0x73, 0x70, 0x65, 0x63, 0x74,
	0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x26, 0x0a, 0x07, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x0c, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x52, 0x07,
	0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x32, 0xc0, 0x02, 0x0a, 0x07, 0x4d, 0x61, 0x63, 0x68,
	0x69, 0x6e, 0x65, 0x12, 0x40, 0x0a, 0x0b, 0x49, 0x6e, 0x69, 0x74, 0x43, 0x6c, 0x75, 0x73, 0x74,
	0x65, 0x72, 0x12, 0x17, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x49, 0x6e, 0x69, 0x74, 0x43, 0x6c, 0x75,
	0x73, 0x74, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x61, 0x70,
	0x69, 0x2e, 0x49, 0x6e, 0x69, 0x74, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3e, 0x0a, 0x0b, 0x4a, 0x6f, 0x69, 0x6e, 0x43, 0x6c, 0x75,
	0x73, 0x74, 0x65, 0x72, 0x12, 0x17, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x4a, 0x6f, 0x69, 0x6e, 0x43,
	0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x33, 0x0a, 0x05, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x16,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x12, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x54, 0x6f, 0x6b,
	0x65, 0x6e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x33, 0x0a, 0x07, 0x49, 0x6e,
	0x73, 0x70, 0x65, 0x63, 0x74, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x10, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x4d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x12,
	0x49, 0x0a, 0x0e, 0x49, 0x6e, 0x73, 0x70, 0x65, 0x63, 0x74, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63,
	0x65, 0x12, 0x1a, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x49, 0x6e, 0x73, 0x70, 0x65, 0x63, 0x74, 0x53,
	0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x49, 0x6e, 0x73, 0x70, 0x65, 0x63, 0x74, 0x53, 0x65, 0x72, 0x76, 0x69,
	0x63, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x37, 0x5a, 0x35, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x70, 0x73, 0x76, 0x69, 0x64, 0x65, 0x72,
	0x73, 0x6b, 0x69, 0x2f, 0x75, 0x6e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x2f, 0x61, 0x70, 0x69,
	0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_machine_api_pb_machine_proto_rawDescOnce sync.Once
	file_internal_machine_api_pb_machine_proto_rawDescData = file_internal_machine_api_pb_machine_proto_rawDesc
)

func file_internal_machine_api_pb_machine_proto_rawDescGZIP() []byte {
	file_internal_machine_api_pb_machine_proto_rawDescOnce.Do(func() {
		file_internal_machine_api_pb_machine_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_machine_api_pb_machine_proto_rawDescData)
	})
	return file_internal_machine_api_pb_machine_proto_rawDescData
}

var file_internal_machine_api_pb_machine_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_internal_machine_api_pb_machine_proto_goTypes = []any{
	(*MachineInfo)(nil),            // 0: api.MachineInfo
	(*NetworkConfig)(nil),          // 1: api.NetworkConfig
	(*InitClusterRequest)(nil),     // 2: api.InitClusterRequest
	(*InitClusterResponse)(nil),    // 3: api.InitClusterResponse
	(*JoinClusterRequest)(nil),     // 4: api.JoinClusterRequest
	(*TokenResponse)(nil),          // 5: api.TokenResponse
	(*Service)(nil),                // 6: api.Service
	(*InspectServiceRequest)(nil),  // 7: api.InspectServiceRequest
	(*InspectServiceResponse)(nil), // 8: api.InspectServiceResponse
	(*Service_Container)(nil),      // 9: api.Service.Container
	(*IPPrefix)(nil),               // 10: api.IPPrefix
	(*IP)(nil),                     // 11: api.IP
	(*IPPort)(nil),                 // 12: api.IPPort
	(*emptypb.Empty)(nil),          // 13: google.protobuf.Empty
}
var file_internal_machine_api_pb_machine_proto_depIdxs = []int32{
	1,  // 0: api.MachineInfo.network:type_name -> api.NetworkConfig
	10, // 1: api.NetworkConfig.subnet:type_name -> api.IPPrefix
	11, // 2: api.NetworkConfig.management_ip:type_name -> api.IP
	12, // 3: api.NetworkConfig.endpoints:type_name -> api.IPPort
	10, // 4: api.InitClusterRequest.network:type_name -> api.IPPrefix
	0,  // 5: api.InitClusterResponse.machine:type_name -> api.MachineInfo
	0,  // 6: api.JoinClusterRequest.machine:type_name -> api.MachineInfo
	0,  // 7: api.JoinClusterRequest.other_machines:type_name -> api.MachineInfo
	9,  // 8: api.Service.containers:type_name -> api.Service.Container
	6,  // 9: api.InspectServiceResponse.service:type_name -> api.Service
	2,  // 10: api.Machine.InitCluster:input_type -> api.InitClusterRequest
	4,  // 11: api.Machine.JoinCluster:input_type -> api.JoinClusterRequest
	13, // 12: api.Machine.Token:input_type -> google.protobuf.Empty
	13, // 13: api.Machine.Inspect:input_type -> google.protobuf.Empty
	7,  // 14: api.Machine.InspectService:input_type -> api.InspectServiceRequest
	3,  // 15: api.Machine.InitCluster:output_type -> api.InitClusterResponse
	13, // 16: api.Machine.JoinCluster:output_type -> google.protobuf.Empty
	5,  // 17: api.Machine.Token:output_type -> api.TokenResponse
	0,  // 18: api.Machine.Inspect:output_type -> api.MachineInfo
	8,  // 19: api.Machine.InspectService:output_type -> api.InspectServiceResponse
	15, // [15:20] is the sub-list for method output_type
	10, // [10:15] is the sub-list for method input_type
	10, // [10:10] is the sub-list for extension type_name
	10, // [10:10] is the sub-list for extension extendee
	0,  // [0:10] is the sub-list for field type_name
}

func init() { file_internal_machine_api_pb_machine_proto_init() }
func file_internal_machine_api_pb_machine_proto_init() {
	if File_internal_machine_api_pb_machine_proto != nil {
		return
	}
	file_internal_machine_api_pb_common_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_internal_machine_api_pb_machine_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*MachineInfo); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*NetworkConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[2].Exporter = func(v any, i int) any {
			switch v := v.(*InitClusterRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[3].Exporter = func(v any, i int) any {
			switch v := v.(*InitClusterResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[4].Exporter = func(v any, i int) any {
			switch v := v.(*JoinClusterRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[5].Exporter = func(v any, i int) any {
			switch v := v.(*TokenResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[6].Exporter = func(v any, i int) any {
			switch v := v.(*Service); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[7].Exporter = func(v any, i int) any {
			switch v := v.(*InspectServiceRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[8].Exporter = func(v any, i int) any {
			switch v := v.(*InspectServiceResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_machine_api_pb_machine_proto_msgTypes[9].Exporter = func(v any, i int) any {
			switch v := v.(*Service_Container); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_internal_machine_api_pb_machine_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_internal_machine_api_pb_machine_proto_goTypes,
		DependencyIndexes: file_internal_machine_api_pb_machine_proto_depIdxs,
		MessageInfos:      file_internal_machine_api_pb_machine_proto_msgTypes,
	}.Build()
	File_internal_machine_api_pb_machine_proto = out.File
	file_internal_machine_api_pb_machine_proto_rawDesc = nil
	file_internal_machine_api_pb_machine_proto_goTypes = nil
	file_internal_machine_api_pb_machine_proto_depIdxs = nil
}
