// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v3.21.12
// source: app/src/dashboard_state.proto

package simulator

import (
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

type DashboardPartitionState struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	CumulativeTimesteps float64   `protobuf:"fixed64,1,opt,name=cumulative_timesteps,json=cumulativeTimesteps,proto3" json:"cumulative_timesteps,omitempty"`
	PartitionName       string    `protobuf:"bytes,2,opt,name=partition_name,json=partitionName,proto3" json:"partition_name,omitempty"`
	State               []float64 `protobuf:"fixed64,3,rep,packed,name=state,proto3" json:"state,omitempty"`
}

func (x *DashboardPartitionState) Reset() {
	*x = DashboardPartitionState{}
	if protoimpl.UnsafeEnabled {
		mi := &file_app_src_dashboard_state_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DashboardPartitionState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DashboardPartitionState) ProtoMessage() {}

func (x *DashboardPartitionState) ProtoReflect() protoreflect.Message {
	mi := &file_app_src_dashboard_state_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DashboardPartitionState.ProtoReflect.Descriptor instead.
func (*DashboardPartitionState) Descriptor() ([]byte, []int) {
	return file_app_src_dashboard_state_proto_rawDescGZIP(), []int{0}
}

func (x *DashboardPartitionState) GetCumulativeTimesteps() float64 {
	if x != nil {
		return x.CumulativeTimesteps
	}
	return 0
}

func (x *DashboardPartitionState) GetPartitionName() string {
	if x != nil {
		return x.PartitionName
	}
	return ""
}

func (x *DashboardPartitionState) GetState() []float64 {
	if x != nil {
		return x.State
	}
	return nil
}

var File_app_src_dashboard_state_proto protoreflect.FileDescriptor

var file_app_src_dashboard_state_proto_rawDesc = []byte{
	0x0a, 0x1d, 0x61, 0x70, 0x70, 0x2f, 0x73, 0x72, 0x63, 0x2f, 0x64, 0x61, 0x73, 0x68, 0x62, 0x6f,
	0x61, 0x72, 0x64, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x89, 0x01, 0x0a, 0x17, 0x44, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x50, 0x61, 0x72,
	0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x31, 0x0a, 0x14, 0x63,
	0x75, 0x6d, 0x75, 0x6c, 0x61, 0x74, 0x69, 0x76, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x65, 0x70, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x01, 0x52, 0x13, 0x63, 0x75, 0x6d, 0x75, 0x6c,
	0x61, 0x74, 0x69, 0x76, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x65, 0x70, 0x73, 0x12, 0x25,
	0x0a, 0x0e, 0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f,
	0x6e, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x03,
	0x20, 0x03, 0x28, 0x01, 0x52, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x42, 0x11, 0x5a, 0x0f, 0x2e,
	0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x73, 0x69, 0x6d, 0x75, 0x6c, 0x61, 0x74, 0x6f, 0x72, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_app_src_dashboard_state_proto_rawDescOnce sync.Once
	file_app_src_dashboard_state_proto_rawDescData = file_app_src_dashboard_state_proto_rawDesc
)

func file_app_src_dashboard_state_proto_rawDescGZIP() []byte {
	file_app_src_dashboard_state_proto_rawDescOnce.Do(func() {
		file_app_src_dashboard_state_proto_rawDescData = protoimpl.X.CompressGZIP(file_app_src_dashboard_state_proto_rawDescData)
	})
	return file_app_src_dashboard_state_proto_rawDescData
}

var file_app_src_dashboard_state_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_app_src_dashboard_state_proto_goTypes = []any{
	(*DashboardPartitionState)(nil), // 0: DashboardPartitionState
}
var file_app_src_dashboard_state_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_app_src_dashboard_state_proto_init() }
func file_app_src_dashboard_state_proto_init() {
	if File_app_src_dashboard_state_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_app_src_dashboard_state_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*DashboardPartitionState); i {
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
			RawDescriptor: file_app_src_dashboard_state_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_app_src_dashboard_state_proto_goTypes,
		DependencyIndexes: file_app_src_dashboard_state_proto_depIdxs,
		MessageInfos:      file_app_src_dashboard_state_proto_msgTypes,
	}.Build()
	File_app_src_dashboard_state_proto = out.File
	file_app_src_dashboard_state_proto_rawDesc = nil
	file_app_src_dashboard_state_proto_goTypes = nil
	file_app_src_dashboard_state_proto_depIdxs = nil
}
