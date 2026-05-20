package services

import (
	"context"

	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/llm/provider/model"
	"github.com/zgiai/ginext/internal/modules/llm/provider/repository"
	"github.com/zgiai/ginext/pkg/logger"
	pb "github.com/zgiai/ginext/pkg/rpc/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ProviderService struct {
	pb.UnimplementedProviderServiceServer
	container *container.ServiceContainer
	repo      repository.ProviderRepository
}

func NewProviderService(c *container.ServiceContainer) *ProviderService {
	db := c.GetDB()
	return &ProviderService{
		container: c,
		repo:      repository.NewProviderRepository(db),
	}
}

func (s *ProviderService) ListProviders(ctx context.Context, req *pb.ListProvidersRequest) (*pb.ListProvidersResponse, error) {
	// Build filters
	var isActive *bool
	if !req.IncludeInactive {
		active := true
		isActive = &active
	}

	// Query providers
	providers, _, err := s.repo.List(ctx, isActive, 0, 1000)
	if err != nil {
		logger.ErrorContext(ctx, "Provider gRPC list failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to list providers: %v", err)
	}

	// Convert to protobuf
	pbProviders := make([]*pb.ProviderInfo, 0, len(providers))
	for _, p := range providers {
		pbProviders = append(pbProviders, s.toPBProvider(p))
	}

	return &pb.ListProvidersResponse{
		Providers: pbProviders,
	}, nil
}

func (s *ProviderService) GetProvider(ctx context.Context, req *pb.GetProviderRequest) (*pb.GetProviderResponse, error) {
	if req.Code == "" {
		return nil, status.Error(codes.InvalidArgument, "provider code is required")
	}

	p, err := s.repo.GetByName(ctx, req.Code)
	if err != nil {
		logger.WarnContext(ctx, "Provider gRPC get failed",
			zap.String("provider", req.Code),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.NotFound, "provider not found: %v", err)
	}

	return &pb.GetProviderResponse{
		Provider: s.toPBProvider(p),
	}, nil
}

func (s *ProviderService) DetectProtocols(ctx context.Context, req *pb.DetectProtocolsRequest) (*pb.DetectProtocolsResponse, error) {
	// This is a placeholder - actual protocol detection would test endpoints
	// For now, return supported protocols based on provider
	p, err := s.repo.GetByName(ctx, req.Provider)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "provider not found: %v", err)
	}

	// Default protocols based on provider name
	supportedProtocols := []string{"openai"}
	if p.Provider == "anthropic" {
		supportedProtocols = []string{"anthropic"}
	}

	return &pb.DetectProtocolsResponse{
		SupportedProtocols:  supportedProtocols,
		RecommendedProtocol: supportedProtocols[0],
		DetectionResults: map[string]string{
			"status": "detection not implemented yet",
		},
	}, nil
}

// toPBProvider converts model.LLMProvider to pb.ProviderInfo
func (s *ProviderService) toPBProvider(p *model.LLMProvider) *pb.ProviderInfo {
	metadata := make(map[string]string)
	if p.Description != "" {
		metadata["description"] = p.Description
	}
	if p.ProviderType != "" {
		metadata["provider_type"] = p.ProviderType
	}
	if p.APIBaseURL != "" {
		metadata["api_base_url"] = p.APIBaseURL
	}

	// Default supported protocols based on provider name
	supportedProtocols := []string{"openai"}
	defaultProtocol := "openai"
	if p.Provider == "anthropic" {
		supportedProtocols = []string{"anthropic"}
		defaultProtocol = "anthropic"
	}

	return &pb.ProviderInfo{
		Code:               p.Provider,
		Name:               p.ProviderName,
		Description:        p.Description,
		SupportedProtocols: supportedProtocols,
		DefaultProtocol:    defaultProtocol,
		Website:            p.APIDocsURL,
		LogoUrl:            p.LogoURL,
		IsActive:           p.IsActive,
		Metadata:           metadata,
	}
}
