// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.27.3
// source: internal/machine/api/pb/common.proto

package pb

import (
	status "google.golang.org/genproto/googleapis/rpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Common metadata message nested in all reply message types, injected by the gRPC proxy to provide information
// about the machine that responded to the request.
type Metadata struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Address of the machine the response came from.
	Machine string `protobuf:"bytes,1,opt,name=machine,proto3" json:"machine,omitempty"`
	// error is set if the request to upstream failed. The rest of the response is undefined.
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
	// error as a gRPC Status message.
	Status *status.Status `protobuf:"bytes,3,opt,name=status,proto3" json:"status,omitempty"`
}

func (x *Metadata) Reset() {
	*x = Metadata{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_common_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Metadata) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Metadata) ProtoMessage() {}

func (x *Metadata) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_common_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Metadata.ProtoReflect.Descriptor instead.
func (*Metadata) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_common_proto_rawDescGZIP(), []int{0}
}

func (x *Metadata) GetMachine() string {
	if x != nil {
		return x.Machine
	}
	return ""
}

func (x *Metadata) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *Metadata) GetStatus() *status.Status {
	if x != nil {
		return x.Status
	}
	return nil
}

// A helper message for marshalling the metadata field and injecting it into the response by the gRPC proxy.
type Empty struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Metadata *Metadata `protobuf:"bytes,1,opt,name=metadata,proto3" json:"metadata,omitempty"`
}

func (x *Empty) Reset() {
	*x = Empty{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_common_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Empty) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Empty) ProtoMessage() {}

func (x *Empty) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_common_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Empty.ProtoReflect.Descriptor instead.
func (*Empty) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_common_proto_rawDescGZIP(), []int{1}
}

func (x *Empty) GetMetadata() *Metadata {
	if x != nil {
		return x.Metadata
	}
	return nil
}

// EmptyResponse is a response message to be returned by the gRPC proxy when a request to the upstream failed.
// The nested Empty.Metadata message should contain an error.
type EmptyResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Messages []*Empty `protobuf:"bytes,1,rep,name=messages,proto3" json:"messages,omitempty"`
}

func (x *EmptyResponse) Reset() {
	*x = EmptyResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_common_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *EmptyResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EmptyResponse) ProtoMessage() {}

func (x *EmptyResponse) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_common_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EmptyResponse.ProtoReflect.Descriptor instead.
func (*EmptyResponse) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_common_proto_rawDescGZIP(), []int{2}
}

func (x *EmptyResponse) GetMessages() []*Empty {
	if x != nil {
		return x.Messages
	}
	return nil
}

type IP struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ip []byte `protobuf:"bytes,1,opt,name=ip,proto3" json:"ip,omitempty"`
}

func (x *IP) Reset() {
	*x = IP{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_common_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IP) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IP) ProtoMessage() {}

func (x *IP) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_common_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IP.ProtoReflect.Descriptor instead.
func (*IP) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_common_proto_rawDescGZIP(), []int{3}
}

func (x *IP) GetIp() []byte {
	if x != nil {
		return x.Ip
	}
	return nil
}

type IPPort struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ip   *IP    `protobuf:"bytes,1,opt,name=ip,proto3" json:"ip,omitempty"`
	Port uint32 `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
}

func (x *IPPort) Reset() {
	*x = IPPort{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_common_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IPPort) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IPPort) ProtoMessage() {}

func (x *IPPort) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_common_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IPPort.ProtoReflect.Descriptor instead.
func (*IPPort) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_common_proto_rawDescGZIP(), []int{4}
}

func (x *IPPort) GetIp() *IP {
	if x != nil {
		return x.Ip
	}
	return nil
}

func (x *IPPort) GetPort() uint32 {
	if x != nil {
		return x.Port
	}
	return 0
}

type IPPrefix struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ip   *IP    `protobuf:"bytes,1,opt,name=ip,proto3" json:"ip,omitempty"`
	Bits uint32 `protobuf:"varint,2,opt,name=bits,proto3" json:"bits,omitempty"`
}

func (x *IPPrefix) Reset() {
	*x = IPPrefix{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_machine_api_pb_common_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IPPrefix) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IPPrefix) ProtoMessage() {}

func (x *IPPrefix) ProtoReflect() protoreflect.Message {
	mi := &file_internal_machine_api_pb_common_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IPPrefix.ProtoReflect.Descriptor instead.
func (*IPPrefix) Descriptor() ([]byte, []int) {
	return file_internal_machine_api_pb_common_proto_rawDescGZIP(), []int{5}
}

func (x *IPPrefix) GetIp() *IP {
	if x != nil {
		return x.Ip
	}
	return nil
}

func (x *IPPrefix) GetBits() uint32 {
	if x != nil {
		return x.Bits
	}
	return 0
}

var File_internal_machine_api_pb_common_proto protoreflect.FileDescriptor

var file_internal_machine_api_pb_common_proto_rawDesc = []byte{
	0x0a, 0x24, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61, 0x63, 0x68, 0x69,
	0x6e, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x62, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03, 0x61, 0x70, 0x69, 0x1a, 0x17, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x72, 0x70, 0x63, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0x66, 0x0a, 0x08, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x12, 0x18, 0x0a, 0x07, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x07, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x72,
	0x72, 0x6f, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72,
	0x12, 0x2a, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x12, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x72, 0x70, 0x63, 0x2e, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0x32, 0x0a, 0x05,
	0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x29, 0x0a, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74,
	0x61, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0d, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x4d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x52, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x22, 0x37, 0x0a, 0x0d, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x26, 0x0a, 0x08, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x0a, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x52,
	0x08, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x22, 0x14, 0x0a, 0x02, 0x49, 0x50, 0x12,
	0x0e, 0x0a, 0x02, 0x69, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x02, 0x69, 0x70, 0x22,
	0x35, 0x0a, 0x06, 0x49, 0x50, 0x50, 0x6f, 0x72, 0x74, 0x12, 0x17, 0x0a, 0x02, 0x69, 0x70, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x07, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x49, 0x50, 0x52, 0x02,
	0x69, 0x70, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d,
	0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x22, 0x37, 0x0a, 0x08, 0x49, 0x50, 0x50, 0x72, 0x65, 0x66,
	0x69, 0x78, 0x12, 0x17, 0x0a, 0x02, 0x69, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x07,
	0x2e, 0x61, 0x70, 0x69, 0x2e, 0x49, 0x50, 0x52, 0x02, 0x69, 0x70, 0x12, 0x12, 0x0a, 0x04, 0x62,
	0x69, 0x74, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x62, 0x69, 0x74, 0x73, 0x42,
	0x37, 0x5a, 0x35, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x70, 0x73,
	0x76, 0x69, 0x64, 0x65, 0x72, 0x73, 0x6b, 0x69, 0x2f, 0x75, 0x6e, 0x63, 0x6c, 0x6f, 0x75, 0x64,
	0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e,
	0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_machine_api_pb_common_proto_rawDescOnce sync.Once
	file_internal_machine_api_pb_common_proto_rawDescData = file_internal_machine_api_pb_common_proto_rawDesc
)

func file_internal_machine_api_pb_common_proto_rawDescGZIP() []byte {
	file_internal_machine_api_pb_common_proto_rawDescOnce.Do(func() {
		file_internal_machine_api_pb_common_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_machine_api_pb_common_proto_rawDescData)
	})
	return file_internal_machine_api_pb_common_proto_rawDescData
}

var file_internal_machine_api_pb_common_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_internal_machine_api_pb_common_proto_goTypes = []any{
	(*Metadata)(nil),      // 0: api.Metadata
	(*Empty)(nil),         // 1: api.Empty
	(*EmptyResponse)(nil), // 2: api.EmptyResponse
	(*IP)(nil),            // 3: api.IP
	(*IPPort)(nil),        // 4: api.IPPort
	(*IPPrefix)(nil),      // 5: api.IPPrefix
	(*status.Status)(nil), // 6: google.rpc.Status
}
var file_internal_machine_api_pb_common_proto_depIdxs = []int32{
	6, // 0: api.Metadata.status:type_name -> google.rpc.Status
	0, // 1: api.Empty.metadata:type_name -> api.Metadata
	1, // 2: api.EmptyResponse.messages:type_name -> api.Empty
	3, // 3: api.IPPort.ip:type_name -> api.IP
	3, // 4: api.IPPrefix.ip:type_name -> api.IP
	5, // [5:5] is the sub-list for method output_type
	5, // [5:5] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_internal_machine_api_pb_common_proto_init() }
func file_internal_machine_api_pb_common_proto_init() {
	if File_internal_machine_api_pb_common_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_machine_api_pb_common_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*Metadata); i {
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
		file_internal_machine_api_pb_common_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*Empty); i {
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
		file_internal_machine_api_pb_common_proto_msgTypes[2].Exporter = func(v any, i int) any {
			switch v := v.(*EmptyResponse); i {
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
		file_internal_machine_api_pb_common_proto_msgTypes[3].Exporter = func(v any, i int) any {
			switch v := v.(*IP); i {
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
		file_internal_machine_api_pb_common_proto_msgTypes[4].Exporter = func(v any, i int) any {
			switch v := v.(*IPPort); i {
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
		file_internal_machine_api_pb_common_proto_msgTypes[5].Exporter = func(v any, i int) any {
			switch v := v.(*IPPrefix); i {
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
			RawDescriptor: file_internal_machine_api_pb_common_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_machine_api_pb_common_proto_goTypes,
		DependencyIndexes: file_internal_machine_api_pb_common_proto_depIdxs,
		MessageInfos:      file_internal_machine_api_pb_common_proto_msgTypes,
	}.Build()
	File_internal_machine_api_pb_common_proto = out.File
	file_internal_machine_api_pb_common_proto_rawDesc = nil
	file_internal_machine_api_pb_common_proto_goTypes = nil
	file_internal_machine_api_pb_common_proto_depIdxs = nil
}
