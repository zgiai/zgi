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
	TransformAuto(ctx context.Context, input ParseArtifactAutoChunkTransformInput) (*ParseArtifactChunkTransformResult, error)
}

type ParseArtifactChunkTransformInput struct {
	TenantID       string
	IndexType      datasetindexing.IndexType
	Artifact       *contracts.ParseArtifact
	ProcessOptions *datasetindexing.ProcessOptions
}

type ParseArtifactAutoChunkTransformInput struct {
	TenantID string
	Artifact *contracts.ParseArtifact
	FileName string
}

type ParseArtifactChunkTransformResult struct {
	Chunks         []dto.TransformedChunk
	IndexType      datasetindexing.IndexType
	ProcessOptions *datasetindexing.ProcessOptions
	Routing        map[string]any
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

func (s *parseArtifactChunkTransformService) TransformAuto(ctx context.Context, input ParseArtifactAutoChunkTransformInput) (*ParseArtifactChunkTransformResult, error) {
	if input.Artifact == nil {
		return nil, ErrParseArtifactRequired
	}
	extractOutput := parseArtifactToExtractOutput(input.Artifact)
	fileName := input.FileName
	if fileName == "" {
		fileName = input.Artifact.FileName
	}

	indexType := datasetindexing.ParagraphIndex
	options := &datasetindexing.ProcessOptions{Mode: "automatic"}
	routing := map[string]any{
		"version":         "v1",
		"matched":         false,
		"fallback_reason": "default paragraph automatic chunking",
	}

	router := datasetindexing.NewRuntimeRouter(ctx, s.llmClient, s.defaultModelSvc, input.TenantID)
	decision, err := router.Route(datasetindexing.RouterInput{
		DataSourceType:  "upload_file",
		DocExt:          fileName,
		ExtractedOutput: extractOutput,
	})
	if err != nil {
		routing["route_error"] = err.Error()
	} else if decision != nil {
		routing["matched"] = decision.Matched
		routing["route_name"] = decision.RouteName
		routing["reason"] = decision.Reason
		for key, value := range decision.RouteMeta {
			routing[key] = value
		}
		if decision.Matched {
			indexType = datasetindexing.IndexType(decision.TargetDocForm)
			options = &datasetindexing.ProcessOptions{
				Mode:        decision.TargetMode,
				ProcessRule: decision.TargetRules,
			}
		}
	}

	chunks, err := s.Transform(ctx, ParseArtifactChunkTransformInput{
		TenantID:       input.TenantID,
		IndexType:      indexType,
		Artifact:       input.Artifact,
		ProcessOptions: options,
	})
	if err != nil {
		return nil, err
	}
	return &ParseArtifactChunkTransformResult{
		Chunks:         chunks,
		IndexType:      indexType,
		ProcessOptions: options,
		Routing:        routing,
	}, nil
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
