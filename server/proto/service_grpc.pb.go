// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.12.4
// source: service.proto

package service_go_proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// KnutServiceClient is the client API for KnutService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type KnutServiceClient interface {
	Hello(ctx context.Context, in *HelloRequest, opts ...grpc.CallOption) (*HelloResponse, error)
}

type knutServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewKnutServiceClient(cc grpc.ClientConnInterface) KnutServiceClient {
	return &knutServiceClient{cc}
}

func (c *knutServiceClient) Hello(ctx context.Context, in *HelloRequest, opts ...grpc.CallOption) (*HelloResponse, error) {
	out := new(HelloResponse)
	err := c.cc.Invoke(ctx, "/knut.service.KnutService/Hello", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// KnutServiceServer is the server API for KnutService service.
// All implementations must embed UnimplementedKnutServiceServer
// for forward compatibility
type KnutServiceServer interface {
	Hello(context.Context, *HelloRequest) (*HelloResponse, error)
	mustEmbedUnimplementedKnutServiceServer()
}

// UnimplementedKnutServiceServer must be embedded to have forward compatible implementations.
type UnimplementedKnutServiceServer struct {
}

func (UnimplementedKnutServiceServer) Hello(context.Context, *HelloRequest) (*HelloResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Hello not implemented")
}
func (UnimplementedKnutServiceServer) mustEmbedUnimplementedKnutServiceServer() {}

// UnsafeKnutServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to KnutServiceServer will
// result in compilation errors.
type UnsafeKnutServiceServer interface {
	mustEmbedUnimplementedKnutServiceServer()
}

func RegisterKnutServiceServer(s grpc.ServiceRegistrar, srv KnutServiceServer) {
	s.RegisterService(&KnutService_ServiceDesc, srv)
}

func _KnutService_Hello_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(HelloRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(KnutServiceServer).Hello(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/knut.service.KnutService/Hello",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(KnutServiceServer).Hello(ctx, req.(*HelloRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// KnutService_ServiceDesc is the grpc.ServiceDesc for KnutService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var KnutService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "knut.service.KnutService",
	HandlerType: (*KnutServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Hello",
			Handler:    _KnutService_Hello_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "service.proto",
}