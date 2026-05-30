package service

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
	datasetindexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	datasetrepository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/pkg/storage"
)

type ParseArtifactChunkTransformService interface {
	Transform(ctx context.Context, input ParseArtifactChunkTransformInput) ([]dto.TransformedChunk, error)
}

type ParseArtifactChunkTransformInput struct {
	TenantID       string
	IndexType      datasetindexing.IndexType
	Artifact       *contracts.ParseArtifact
	ProcessOptions *datasetindexing.ProcessOptions
}

type parseArtifactChunkTransformService struct {
	storage         storage.Storage
	documentRepo    datasetrepository.DocumentRepository
	defaultModelSvc llmdefaultservice.DefaultModelService
	llmClient       llmclient.LLMClient
}

func NewParseArtifactChunkTransformService(
	storage storage.Storage,
	documentRepo datasetrepository.DocumentRepository,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	llmClient llmclient.LLMClient,
) ParseArtifactChunkTransformService {
	return &parseArtifactChunkTransformService{
		storage:         storage,
		documentRepo:    documentRepo,
		defaultModelSvc: defaultModelSvc,
		llmClient:       llmClient,
	}
}

func (s *parseArtifactChunkTransformService) Transform(ctx context.Context, input ParseArtifactChunkTransformInput) ([]dto.TransformedChunk, error) {
	if input.Artifact == nil {
		return nil, ErrParseArtifactRequired
	}
	extractOutput := parseArtifactToExtractOutput(input.Artifact)
	factory := datasetindexing.NewIndexProcessorFactory(
		input.IndexType,
		s.storage,
		s.documentRepo,
		s.defaultModelSvc,
		s.llmClient,
		input.TenantID,
	)
	processor, err := factory.CreateIndexProcessor()
	if err != nil {
		return nil, fmt.Errorf("create index processor: %w", err)
	}
	options := input.ProcessOptions
	if options == nil {
		options = &datasetindexing.ProcessOptions{}
	}
	return processor.Transform(ctx, extractOutput, options)
}

func parseArtifactToExtractOutput(artifact *contracts.ParseArtifact) *dto.ExtractOutput {
	if artifact == nil {
		return nil
	}
	output := &dto.ExtractOutput{
		Markdown: artifact.Markdown,
		Source:   string(artifact.SourceType),
		Metadata: map[string]any{},
		Elements: make([]dto.ExtractElement, 0, len(artifact.Elements)),
	}
	for key, value := range artifact.Metadata {
		output.Metadata[key] = value
	}
	for _, element := range artifact.Elements {
		output.Elements = append(output.Elements, dto.ExtractElement{
			Type:      element.Type,
			Subtype:   element.Subtype,
			Page:      element.Page,
			Content:   element.Content,
			BBox:      parseArtifactBoundingBoxToExtract(element.BBox),
			Ordinal:   element.Ordinal,
			Precision: element.Precision,
			Metadata:  cloneAnyMap(element.Metadata),
		})
	}
	return output
}

func parseArtifactBoundingBoxToExtract(box *contracts.ParseBoundingBox) *dto.ExtractBoundingBox {
	if box == nil {
		return nil
	}
	return &dto.ExtractBoundingBox{
		Left:   box.Left,
		Top:    box.Top,
		Right:  box.Right,
		Bottom: box.Bottom,
	}
}
