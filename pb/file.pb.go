// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        v5.29.3
// source: file.proto

package pb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type FileID struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	FileID        string                 `protobuf:"bytes,1,opt,name=fileID" json:"fileID,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *FileID) Reset() {
	*x = FileID{}
	mi := &file_file_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *FileID) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileID) ProtoMessage() {}

func (x *FileID) ProtoReflect() protoreflect.Message {
	mi := &file_file_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileID.ProtoReflect.Descriptor instead.
func (*FileID) Descriptor() ([]byte, []int) {
	return file_file_proto_rawDescGZIP(), []int{0}
}

func (x *FileID) GetFileID() string {
	if x != nil {
		return x.FileID
	}
	return ""
}

type FileContent struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=name" json:"name,omitempty"`
	Content       []byte                 `protobuf:"bytes,2,opt,name=content" json:"content,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *FileContent) Reset() {
	*x = FileContent{}
	mi := &file_file_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *FileContent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileContent) ProtoMessage() {}

func (x *FileContent) ProtoReflect() protoreflect.Message {
	mi := &file_file_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileContent.ProtoReflect.Descriptor instead.
func (*FileContent) Descriptor() ([]byte, []int) {
	return file_file_proto_rawDescGZIP(), []int{1}
}

func (x *FileContent) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *FileContent) GetContent() []byte {
	if x != nil {
		return x.Content
	}
	return nil
}

type FileListType struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	FileIDs       map[string]string      `protobuf:"bytes,1,rep,name=fileIDs" json:"fileIDs,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *FileListType) Reset() {
	*x = FileListType{}
	mi := &file_file_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *FileListType) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FileListType) ProtoMessage() {}

func (x *FileListType) ProtoReflect() protoreflect.Message {
	mi := &file_file_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FileListType.ProtoReflect.Descriptor instead.
func (*FileListType) Descriptor() ([]byte, []int) {
	return file_file_proto_rawDescGZIP(), []int{2}
}

func (x *FileListType) GetFileIDs() map[string]string {
	if x != nil {
		return x.FileIDs
	}
	return nil
}

var File_file_proto protoreflect.FileDescriptor

var file_file_proto_rawDesc = string([]byte{
	0x0a, 0x0a, 0x66, 0x69, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x02, 0x70, 0x62,
	0x22, 0x20, 0x0a, 0x06, 0x46, 0x69, 0x6c, 0x65, 0x49, 0x44, 0x12, 0x16, 0x0a, 0x06, 0x66, 0x69,
	0x6c, 0x65, 0x49, 0x44, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x66, 0x69, 0x6c, 0x65,
	0x49, 0x44, 0x22, 0x3b, 0x0a, 0x0b, 0x46, 0x69, 0x6c, 0x65, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e,
	0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x22,
	0x83, 0x01, 0x0a, 0x0c, 0x46, 0x69, 0x6c, 0x65, 0x4c, 0x69, 0x73, 0x74, 0x54, 0x79, 0x70, 0x65,
	0x12, 0x37, 0x0a, 0x07, 0x66, 0x69, 0x6c, 0x65, 0x49, 0x44, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1d, 0x2e, 0x70, 0x62, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x4c, 0x69, 0x73, 0x74, 0x54,
	0x79, 0x70, 0x65, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x49, 0x44, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79,
	0x52, 0x07, 0x66, 0x69, 0x6c, 0x65, 0x49, 0x44, 0x73, 0x1a, 0x3a, 0x0a, 0x0c, 0x46, 0x69, 0x6c,
	0x65, 0x49, 0x44, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x3a, 0x02, 0x38, 0x01, 0x42, 0x24, 0x5a, 0x1d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e,
	0x63, 0x6f, 0x6d, 0x2f, 0x63, 0x72, 0x69, 0x79, 0x6c, 0x65, 0x2f, 0x67, 0x6f, 0x2d, 0x6a, 0x75,
	0x64, 0x67, 0x65, 0x2f, 0x70, 0x62, 0x92, 0x03, 0x02, 0x08, 0x02, 0x62, 0x08, 0x65, 0x64, 0x69,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x70, 0xe8, 0x07,
})

var (
	file_file_proto_rawDescOnce sync.Once
	file_file_proto_rawDescData []byte
)

func file_file_proto_rawDescGZIP() []byte {
	file_file_proto_rawDescOnce.Do(func() {
		file_file_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_file_proto_rawDesc), len(file_file_proto_rawDesc)))
	})
	return file_file_proto_rawDescData
}

var file_file_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_file_proto_goTypes = []any{
	(*FileID)(nil),       // 0: pb.FileID
	(*FileContent)(nil),  // 1: pb.FileContent
	(*FileListType)(nil), // 2: pb.FileListType
	nil,                  // 3: pb.FileListType.FileIDsEntry
}
var file_file_proto_depIdxs = []int32{
	3, // 0: pb.FileListType.fileIDs:type_name -> pb.FileListType.FileIDsEntry
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_file_proto_init() }
func file_file_proto_init() {
	if File_file_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_file_proto_rawDesc), len(file_file_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_file_proto_goTypes,
		DependencyIndexes: file_file_proto_depIdxs,
		MessageInfos:      file_file_proto_msgTypes,
	}.Build()
	File_file_proto = out.File
	file_file_proto_goTypes = nil
	file_file_proto_depIdxs = nil
}
