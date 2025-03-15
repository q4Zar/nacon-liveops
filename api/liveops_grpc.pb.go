// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v3.21.12
// source: api/liveops.proto

package api

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

const (
	LiveOpsService_CreateEvent_FullMethodName = "/liveops.LiveOpsService/CreateEvent"
	LiveOpsService_UpdateEvent_FullMethodName = "/liveops.LiveOpsService/UpdateEvent"
	LiveOpsService_DeleteEvent_FullMethodName = "/liveops.LiveOpsService/DeleteEvent"
	LiveOpsService_ListEvents_FullMethodName  = "/liveops.LiveOpsService/ListEvents"
)

// LiveOpsServiceClient is the client API for LiveOpsService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type LiveOpsServiceClient interface {
	CreateEvent(ctx context.Context, in *EventRequest, opts ...grpc.CallOption) (*EventResponse, error)
	UpdateEvent(ctx context.Context, in *EventRequest, opts ...grpc.CallOption) (*EventResponse, error)
	DeleteEvent(ctx context.Context, in *DeleteRequest, opts ...grpc.CallOption) (*DeleteResponse, error)
	ListEvents(ctx context.Context, in *Empty, opts ...grpc.CallOption) (*EventsResponse, error)
}

type liveOpsServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewLiveOpsServiceClient(cc grpc.ClientConnInterface) LiveOpsServiceClient {
	return &liveOpsServiceClient{cc}
}

func (c *liveOpsServiceClient) CreateEvent(ctx context.Context, in *EventRequest, opts ...grpc.CallOption) (*EventResponse, error) {
	out := new(EventResponse)
	err := c.cc.Invoke(ctx, LiveOpsService_CreateEvent_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *liveOpsServiceClient) UpdateEvent(ctx context.Context, in *EventRequest, opts ...grpc.CallOption) (*EventResponse, error) {
	out := new(EventResponse)
	err := c.cc.Invoke(ctx, LiveOpsService_UpdateEvent_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *liveOpsServiceClient) DeleteEvent(ctx context.Context, in *DeleteRequest, opts ...grpc.CallOption) (*DeleteResponse, error) {
	out := new(DeleteResponse)
	err := c.cc.Invoke(ctx, LiveOpsService_DeleteEvent_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *liveOpsServiceClient) ListEvents(ctx context.Context, in *Empty, opts ...grpc.CallOption) (*EventsResponse, error) {
	out := new(EventsResponse)
	err := c.cc.Invoke(ctx, LiveOpsService_ListEvents_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LiveOpsServiceServer is the server API for LiveOpsService service.
// All implementations must embed UnimplementedLiveOpsServiceServer
// for forward compatibility
type LiveOpsServiceServer interface {
	CreateEvent(context.Context, *EventRequest) (*EventResponse, error)
	UpdateEvent(context.Context, *EventRequest) (*EventResponse, error)
	DeleteEvent(context.Context, *DeleteRequest) (*DeleteResponse, error)
	ListEvents(context.Context, *Empty) (*EventsResponse, error)
	mustEmbedUnimplementedLiveOpsServiceServer()
}

// UnimplementedLiveOpsServiceServer must be embedded to have forward compatible implementations.
type UnimplementedLiveOpsServiceServer struct {
}

func (UnimplementedLiveOpsServiceServer) CreateEvent(context.Context, *EventRequest) (*EventResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateEvent not implemented")
}
func (UnimplementedLiveOpsServiceServer) UpdateEvent(context.Context, *EventRequest) (*EventResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateEvent not implemented")
}
func (UnimplementedLiveOpsServiceServer) DeleteEvent(context.Context, *DeleteRequest) (*DeleteResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteEvent not implemented")
}
func (UnimplementedLiveOpsServiceServer) ListEvents(context.Context, *Empty) (*EventsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListEvents not implemented")
}
func (UnimplementedLiveOpsServiceServer) mustEmbedUnimplementedLiveOpsServiceServer() {}

// UnsafeLiveOpsServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to LiveOpsServiceServer will
// result in compilation errors.
type UnsafeLiveOpsServiceServer interface {
	mustEmbedUnimplementedLiveOpsServiceServer()
}

func RegisterLiveOpsServiceServer(s grpc.ServiceRegistrar, srv LiveOpsServiceServer) {
	s.RegisterService(&LiveOpsService_ServiceDesc, srv)
}

func _LiveOpsService_CreateEvent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LiveOpsServiceServer).CreateEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: LiveOpsService_CreateEvent_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LiveOpsServiceServer).CreateEvent(ctx, req.(*EventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _LiveOpsService_UpdateEvent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LiveOpsServiceServer).UpdateEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: LiveOpsService_UpdateEvent_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LiveOpsServiceServer).UpdateEvent(ctx, req.(*EventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _LiveOpsService_DeleteEvent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LiveOpsServiceServer).DeleteEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: LiveOpsService_DeleteEvent_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LiveOpsServiceServer).DeleteEvent(ctx, req.(*DeleteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _LiveOpsService_ListEvents_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LiveOpsServiceServer).ListEvents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: LiveOpsService_ListEvents_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LiveOpsServiceServer).ListEvents(ctx, req.(*Empty))
	}
	return interceptor(ctx, in, info, handler)
}

// LiveOpsService_ServiceDesc is the grpc.ServiceDesc for LiveOpsService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var LiveOpsService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "liveops.LiveOpsService",
	HandlerType: (*LiveOpsServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateEvent",
			Handler:    _LiveOpsService_CreateEvent_Handler,
		},
		{
			MethodName: "UpdateEvent",
			Handler:    _LiveOpsService_UpdateEvent_Handler,
		},
		{
			MethodName: "DeleteEvent",
			Handler:    _LiveOpsService_DeleteEvent_Handler,
		},
		{
			MethodName: "ListEvents",
			Handler:    _LiveOpsService_ListEvents_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "api/liveops.proto",
}
