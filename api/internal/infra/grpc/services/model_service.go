package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ModelService struct {
	pb.UnimplementedModelServiceServer
	container *container.ServiceContainer
	repo      repository.ModelRepository
}

func NewModelService(c *container.ServiceContainer) *ModelService {
	db := c.GetDB()
	return &ModelService{
		container: c,
		repo:      repository.NewModelRepository(db),
	}
}

func (s *ModelService) ListModels(ctx context.Context, req *pb.ListModelsRequest) (*pb.ListModelsResponse, error) {
	// Calculate offset from page
	offset := 0
	limit := int(req.PageSize)
	if req.Page > 0 {
		offset = (int(req.Page) - 1) * limit
	}
	if limit <= 0 {
		limit = 20 // Default page size
	}

	// Build filters
	var isActive *bool
	if req.IsActive {
		active := true
		isActive = &active
	}

	// Query models.
	// Keep the legacy protobuf field empty; protocol is no longer part of channel/model routing.
	models, total, err := s.repo.List(ctx, nil, req.Provider, "", isActive, offset, limit)
	if err != nil {
		logger.ErrorContext(ctx, "Model gRPC list failed",
			zap.String("provider", req.Provider),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to list models: %v", err)
	}

	// Convert to protobuf
	pbModels := make([]*pb.ModelInfo, 0, len(models))
	for _, m := range models {
		pbModel, err := s.toPBModel(m)
		if err != nil {
			logger.WarnContext(ctx, "Model gRPC conversion failed",
				zap.Stringer("model_id", m.ID),
				zap.String("model", m.Model),
				zap.String("provider", m.Provider),
				zap.Error(err),
			)
			continue
		}
		pbModels = append(pbModels, pbModel)
	}

	return &pb.ListModelsResponse{
		Models: pbModels,
		Total:  total,
	}, nil
}

func (s *ModelService) GetModel(ctx context.Context, req *pb.GetModelRequest) (*pb.GetModelResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "model id is required")
	}

	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid model ID: %v", err)
	}

	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		logger.WarnContext(ctx, "Model gRPC get failed",
			zap.String("model_id", req.Id),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.NotFound, "model not found: %v", err)
	}

	pbModel, err := s.toPBModel(m)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert model: %v", err)
	}

	return &pb.GetModelResponse{
		Model: pbModel,
	}, nil
}

func (s *ModelService) SyncModels(ctx context.Context, req *pb.SyncModelsRequest) (*pb.SyncModelsResponse, error) {
	// This is a placeholder - actual sync logic would call provider APIs
	// For now, just return a success response
	return &pb.SyncModelsResponse{
		SyncedCount:  0,
		UpdatedCount: 0,
		FailedCount:  0,
		Errors:       []string{"sync not implemented yet"},
	}, nil
}

// toPBModel converts model.LLMModel to pb.ModelInfo
func (s *ModelService) toPBModel(m *model.LLMModel) (*pb.ModelInfo, error) {
	// Convert pricing from decimal to string
	pricingInput := ""
	if !m.InputPrice.IsZero() {
		pricingInput = m.InputPrice.String()
	}
	pricingOutput := ""
	if !m.OutputPrice.IsZero() {
		pricingOutput = m.OutputPrice.String()
	}

	// Build metadata
	metadata := make(map[string]string)
	if m.Description != "" {
		metadata["description"] = m.Description
	}
	if m.Family != "" {
		metadata["family"] = m.Family
	}
	if m.Status != "" {
		metadata["status"] = m.Status
	}
	if m.KnowledgeCutoff != "" {
		metadata["knowledge_cutoff"] = m.KnowledgeCutoff
	}

	return &pb.ModelInfo{
		Id:                   m.ID.String(),
		ModelName:            m.Model,
		DisplayName:          m.ModelName,
		Provider:             m.Provider,
		Protocol:             "",
		UseCases:             m.UseCases,
		MaxTokens:            int32(m.ContextWindow),
		MaxOutputTokens:      int32(m.MaxOutputTokens),
		SupportsStreaming:    m.SupportsStreaming,
		SupportsFunctionCall: m.SupportsToolCall,
		SupportsVision:       m.SupportsVision,
		IsActive:             m.IsActive,
		PricingInput:         pricingInput,
		PricingOutput:        pricingOutput,
		Metadata:             metadata,
		CreatedAt:            m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:            m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}
