package pb

import (
	"context"
	"google.golang.org/grpc"
)

type StartAgentRequest struct {
	Workspace string `protobuf:"bytes,1,opt,name=workspace,proto3" json:"workspace,omitempty"`
	Task      string `protobuf:"bytes,2,opt,name=task,proto3" json:"task,omitempty"`
	Session   string `protobuf:"bytes,3,opt,name=session,proto3" json:"session,omitempty"`
}

func (x *StartAgentRequest) Reset() { *x = StartAgentRequest{} }
func (x *StartAgentRequest) String() string { return "" }
func (x *StartAgentRequest) ProtoMessage() {}

type StartAgentResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Error   string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *StartAgentResponse) Reset() { *x = StartAgentResponse{} }
func (x *StartAgentResponse) String() string { return "" }
func (x *StartAgentResponse) ProtoMessage() {}

type StopAgentRequest struct {
	Workspace string `protobuf:"bytes,1,opt,name=workspace,proto3" json:"workspace,omitempty"`
}

func (x *StopAgentRequest) Reset() { *x = StopAgentRequest{} }
func (x *StopAgentRequest) String() string { return "" }
func (x *StopAgentRequest) ProtoMessage() {}

type StopAgentResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Error   string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *StopAgentResponse) Reset() { *x = StopAgentResponse{} }
func (x *StopAgentResponse) String() string { return "" }
func (x *StopAgentResponse) ProtoMessage() {}

type StreamEventsRequest struct {
	Workspace string `protobuf:"bytes,1,opt,name=workspace,proto3" json:"workspace,omitempty"`
}

func (x *StreamEventsRequest) Reset() { *x = StreamEventsRequest{} }
func (x *StreamEventsRequest) String() string { return "" }
func (x *StreamEventsRequest) ProtoMessage() {}

type AgentEvent struct {
	Type    string `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"`
	Role    string `protobuf:"bytes,2,opt,name=role,proto3" json:"role,omitempty"`
	Content string `protobuf:"bytes,3,opt,name=content,proto3" json:"content,omitempty"`
	Success bool   `protobuf:"varint,4,opt,name=success,proto3" json:"success,omitempty"`
	Stream  bool   `protobuf:"varint,5,opt,name=stream,proto3" json:"stream,omitempty"`
	Error   string `protobuf:"bytes,6,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *AgentEvent) Reset() { *x = AgentEvent{} }
func (x *AgentEvent) String() string { return "" }
func (x *AgentEvent) ProtoMessage() {}

type RespondPermissionRequest struct {
	Workspace string `protobuf:"bytes,1,opt,name=workspace,proto3" json:"workspace,omitempty"`
	Approved  bool   `protobuf:"varint,2,opt,name=approved,proto3" json:"approved,omitempty"`
	Remember  bool   `protobuf:"varint,3,opt,name=remember,proto3" json:"remember,omitempty"`
}

func (x *RespondPermissionRequest) Reset() { *x = RespondPermissionRequest{} }
func (x *RespondPermissionRequest) String() string { return "" }
func (x *RespondPermissionRequest) ProtoMessage() {}

type RespondPermissionResponse struct {
	Success bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Error   string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *RespondPermissionResponse) Reset() { *x = RespondPermissionResponse{} }
func (x *RespondPermissionResponse) String() string { return "" }
func (x *RespondPermissionResponse) ProtoMessage() {}

type AgentServiceServer interface {
	StartAgent(context.Context, *StartAgentRequest) (*StartAgentResponse, error)
	StopAgent(context.Context, *StopAgentRequest) (*StopAgentResponse, error)
	StreamEvents(*StreamEventsRequest, AgentService_StreamEventsServer) error
	RespondPermission(context.Context, *RespondPermissionRequest) (*RespondPermissionResponse, error)
}

type AgentService_StreamEventsServer interface {
	Send(*AgentEvent) error
	grpc.ServerStream
}

type agentServiceStreamEventsServer struct {
	grpc.ServerStream
}

func (x *agentServiceStreamEventsServer) Send(m *AgentEvent) error {
	return x.ServerStream.SendMsg(m)
}

func RegisterAgentServiceServer(s grpc.ServiceRegistrar, srv AgentServiceServer) {
	s.RegisterService(&AgentService_ServiceDesc, srv)
}

func _AgentService_StartAgent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartAgentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServiceServer).StartAgent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/daemon.AgentService/StartAgent",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServiceServer).StartAgent(ctx, req.(*StartAgentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AgentService_StopAgent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopAgentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServiceServer).StopAgent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/daemon.AgentService/StopAgent",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServiceServer).StopAgent(ctx, req.(*StopAgentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AgentService_RespondPermission_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RespondPermissionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AgentServiceServer).RespondPermission(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/daemon.AgentService/RespondPermission",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AgentServiceServer).RespondPermission(ctx, req.(*RespondPermissionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AgentService_StreamEvents_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(StreamEventsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(AgentServiceServer).StreamEvents(m, &agentServiceStreamEventsServer{stream})
}

var AgentService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "daemon.AgentService",
	HandlerType: (*AgentServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "StartAgent",
			Handler:    _AgentService_StartAgent_Handler,
		},
		{
			MethodName: "StopAgent",
			Handler:    _AgentService_StopAgent_Handler,
		},
		{
			MethodName: "RespondPermission",
			Handler:    _AgentService_RespondPermission_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamEvents",
			Handler:       _AgentService_StreamEvents_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "daemon.proto",
}
