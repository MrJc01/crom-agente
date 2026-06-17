package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/crom/crom-agente/internal/daemon/pb"
	"github.com/crom/crom-agente/internal/orchestrator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	manager        *orchestrator.MultiAgentManager
	server         *grpc.Server
	router         *AgentEventsRouter
	activeHandlers map[string]*daemonAPIEventHandler
	mu             sync.Mutex
	SessionToken   string
}

func NewGRPCServer(manager *orchestrator.MultiAgentManager, router *AgentEventsRouter) *GRPCServer {
	return &GRPCServer{
		manager:        manager,
		router:         router,
		activeHandlers: make(map[string]*daemonAPIEventHandler),
	}
}

func (s *GRPCServer) Start(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("falha ao ligar gRPC Server na porta %d: %w", port, err)
	}

	s.server = grpc.NewServer()
	pb.RegisterAgentServiceServer(s.server, s)

	go func() {
		if err := s.server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			log.Printf("[GRPCServer] Erro no servidor gRPC: %v", err)
		}
	}()

	log.Printf("[GRPCServer] Servidor gRPC iniciado em %s", addr)
	return nil
}

func (s *GRPCServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

func (s *GRPCServer) authorize(ctx context.Context) error {
	if s.SessionToken == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "metadata ausente")
	}
	tokens := md.Get("authorization")
	if len(tokens) == 0 {
		tokens = md.Get("x-session-token")
	}
	if len(tokens) == 0 {
		return status.Errorf(codes.Unauthenticated, "token de autorizacao ausente")
	}
	token := tokens[0]
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = token[7:]
	}
	if token != s.SessionToken {
		return status.Errorf(codes.Unauthenticated, "token de autorizacao invalido")
	}
	return nil
}

func (s *GRPCServer) StartAgent(ctx context.Context, req *pb.StartAgentRequest) (*pb.StartAgentResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	handler := &daemonAPIEventHandler{
		workspaceName: req.Workspace,
		router:        s.router,
		permRespChan:  make(chan permissionResult, 1),
	}
	handler.onFinished = func() {
		s.mu.Lock()
		delete(s.activeHandlers, req.Workspace)
		s.mu.Unlock()
	}
	s.activeHandlers[req.Workspace] = handler
	s.mu.Unlock()

	err := s.manager.StartAgent(context.Background(), req.Workspace, req.Session, req.Task, handler)
	if err != nil {
		s.mu.Lock()
		delete(s.activeHandlers, req.Workspace)
		s.mu.Unlock()
		return &pb.StartAgentResponse{Success: false, Error: err.Error()}, nil
	}

	return &pb.StartAgentResponse{Success: true}, nil
}

func (s *GRPCServer) StopAgent(ctx context.Context, req *pb.StopAgentRequest) (*pb.StopAgentResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	err := s.manager.StopAgent(req.Workspace)
	if err != nil {
		return &pb.StopAgentResponse{Success: false, Error: err.Error()}, nil
	}
	return &pb.StopAgentResponse{Success: true}, nil
}

func (s *GRPCServer) RespondPermission(ctx context.Context, req *pb.RespondPermissionRequest) (*pb.RespondPermissionResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	h, exists := s.activeHandlers[req.Workspace]
	s.mu.Unlock()

	if !exists {
		return &pb.RespondPermissionResponse{Success: false, Error: "nenhuma sessao ativa aguardando permissao"}, nil
	}

	select {
	case h.permRespChan <- permissionResult{approved: req.Approved, remember: req.Remember}:
		return &pb.RespondPermissionResponse{Success: true}, nil
	default:
		return &pb.RespondPermissionResponse{Success: false, Error: "canal de permissao bloqueado ou cheio"}, nil
	}
}

func (s *GRPCServer) StreamEvents(req *pb.StreamEventsRequest, stream pb.AgentService_StreamEventsServer) error {
	if err := s.authorize(stream.Context()); err != nil {
		return err
	}
	eventCh := make(chan IPCResponse, 100)
	s.router.Register(req.Workspace, eventCh)
	defer s.router.Unregister(req.Workspace, eventCh)

	ctx := stream.Context()
	for {
		select {
		case resp := <-eventCh:
			var payload map[string]interface{}
			if len(resp.Data) > 0 {
				_ = json.Unmarshal(resp.Data, &payload)
			}

			evt := &pb.AgentEvent{
				Success: resp.Success,
				Stream:  resp.Stream,
				Error:   resp.Error,
			}
			if payload != nil {
				if t, ok := payload["type"].(string); ok {
					evt.Type = t
				}
				if r, ok := payload["role"].(string); ok {
					evt.Role = r
				}
				if c, ok := payload["content"].(string); ok {
					evt.Content = c
				}
			}

			if err := stream.Send(evt); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
