package grpc

import (
	"fmt"
	"net"

	"github.com/zgiai/zgi/api/internal/infra/grpc/services"
	modelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"
)

type Server struct {
	grpcServer *grpc.Server
}

func NewServer(db *gorm.DB) *Server {
	s := grpc.NewServer(observability.GRPCServerOptions()...)

	modelService := services.NewModelService(modelrepo.NewModelRepository(db))
	pb.RegisterModelServiceServer(s, modelService)

	providerService := services.NewProviderService(providerrepo.NewProviderRepository(db))
	pb.RegisterProviderServiceServer(s, providerService)

	reflection.Register(s)
	return &Server{
		grpcServer: s,
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
