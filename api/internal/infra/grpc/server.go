package grpc

import (
	"fmt"
	"net"

	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/infra/grpc/services"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Server struct {
	grpcServer *grpc.Server
	container  *container.ServiceContainer
}

func NewServer(c *container.ServiceContainer) *Server {
	s := grpc.NewServer(observability.GRPCServerOptions()...)

	// Register services
	modelService := services.NewModelService(c)
	pb.RegisterModelServiceServer(s, modelService)

	providerService := services.NewProviderService(c)
	pb.RegisterProviderServiceServer(s, providerService)

	reflection.Register(s)
	return &Server{
		grpcServer: s,
		container:  c,
	}
}

func (s *Server) Serve(listener net.Listener) error {
	logger.Info("Starting gRPC server", "listen_addr", listener.Addr().String())
	return s.grpcServer.Serve(listener)
}

func (s *Server) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	return s.Serve(lis)
}

func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}
