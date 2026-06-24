package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datasetindexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
)

var (
	ErrDocumentChunkEmbeddingsRequired = errors.New("document chunk embeddings are required")
	ErrEmbeddingServiceRequired        = errors.New("embedding service is required")
)

const documentChunkEmbeddingBatchSize = 8

type DocumentChunkEmbeddingService interface {
	GenerateEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error)
	GenerateAdditionalEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error)
	GenerateChunkEmbedding(ctx context.Context, input GenerateDocumentChunkEmbeddingInput) (*model.DocumentChunkEmbedding, error)
}

type GenerateDocumentChunkEmbeddingsInput struct {
	OrganizationID    string
	AssetID           uuid.UUID
	ProcessingRunID   uuid.UUID
	GenerationNo      int64
	EmbeddingProvider string
	EmbeddingModel    string
	RequestedBy       string
	Chunks            []*model.DocumentChunk
}

type GenerateDocumentChunkEmbeddingsResult struct {
	Embeddings         []*model.DocumentChunkEmbedding `json:"embeddings"`
	EmbeddingCount     int                             `json:"embedding_count"`
	EmbeddingProvider  string                          `json:"embedding_provider"`
	EmbeddingModel     string                          `json:"embedding_model"`
	EmbeddingDimension int                             `json:"embedding_dimension"`
}

type GenerateDocumentChunkEmbeddingInput struct {
	OrganizationID    string
	AssetID           uuid.UUID
	ProcessingRunID   uuid.UUID
	GenerationNo      int64
	EmbeddingProvider string
	EmbeddingModel    string
	RequestedBy       string
	Chunk             *model.DocumentChunk
}

type DocumentChunkEmbeddingServiceOption func(*documentChunkEmbeddingService)

type DocumentChunkEmbeddingFactory func(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error)

type documentChunkEmbeddingService struct {
	assets           repository.DocumentAssetRepository
	embeddings       repository.DocumentChunkEmbeddingRepository
	llmClient        llmclient.LLMClient
	defaultModelSvc  llmdefaultservice.DefaultModelService
	embeddingFactory DocumentChunkEmbeddingFactory
	vectorIndex      FileAssetVectorIndexService
}

func NewDocumentChunkEmbeddingService(
	assets repository.DocumentAssetRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	options ...DocumentChunkEmbeddingServiceOption,
) DocumentChunkEmbeddingService {
	svc := &documentChunkEmbeddingService{
		assets:          assets,
		embeddings:      embeddings,
		llmClient:       llmClient,
		defaultModelSvc: defaultModelSvc,
	}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	return svc
}

func WithDocumentChunkEmbeddingFactory(factory DocumentChunkEmbeddingFactory) DocumentChunkEmbeddingServiceOption {
	return func(s *documentChunkEmbeddingService) {
		s.embeddingFactory = factory
	}
}

func WithDocumentChunkVectorIndex(vectorIndex FileAssetVectorIndexService) DocumentChunkEmbeddingServiceOption {
	return func(s *documentChunkEmbeddingService) {
		s.vectorIndex = vectorIndex
	}
}

func (s *documentChunkEmbeddingService) GenerateEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error) {
	return s.generateEmbeddings(ctx, input, true, true)
}

func (s *documentChunkEmbeddingService) GenerateAdditionalEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error) {
	return s.generateEmbeddings(ctx, input, false, false)
}

func (s *documentChunkEmbeddingService) GenerateChunkEmbedding(ctx context.Context, input GenerateDocumentChunkEmbeddingInput) (*model.DocumentChunkEmbedding, error) {
	if input.Chunk == nil {
		return nil, ErrDocumentChunkEmbeddingsRequired
	}
	result, err := s.generateEmbeddings(ctx, GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    input.OrganizationID,
		AssetID:           input.AssetID,
		ProcessingRunID:   input.ProcessingRunID,
		GenerationNo:      input.GenerationNo,
		EmbeddingProvider: input.EmbeddingProvider,
		EmbeddingModel:    input.EmbeddingModel,
		RequestedBy:       input.RequestedBy,
		Chunks:            []*model.DocumentChunk{input.Chunk},
	}, false, true)
	if err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 {
		return nil, ErrDocumentChunkEmbeddingsRequired
	}
	return result.Embeddings[0], nil
}

func (s *documentChunkEmbeddingService) generateEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, clearExisting bool, indexFileVector bool) (*GenerateDocumentChunkEmbeddingsResult, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.ProcessingRunID == uuid.Nil || input.GenerationNo <= 0 {
		return nil, ErrProcessingRunMismatch
	}
	leafChunks := leafDocumentChunks(input.Chunks)
	if len(leafChunks) == 0 {
		return nil, ErrDocumentChunkEmbeddingsRequired
	}
	asset, err := s.assets.GetAssetByID(ctx, input.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != input.OrganizationID {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.ProcessingRunID == nil ||
		*asset.ProcessingRunID != input.ProcessingRunID ||
		asset.GenerationNo != input.GenerationNo {
		return nil, ErrProcessingRunMismatch
	}
	if clearExisting {
		if err := s.embeddings.DeleteByAsset(ctx, input.OrganizationID, input.AssetID); err != nil {
			return nil, err
		}
		if s.vectorIndex != nil {
			if err := s.vectorIndex.DeleteAssetIndex(ctx, asset); err != nil {
				return nil, fmt.Errorf("delete document chunk vectors: %w", err)
			}
		}
	}

	resolvedProvider, resolvedModel, err := s.resolveEmbeddingModel(ctx, input)
	if err != nil {
		return nil, err
	}
	embeddingSvc, err := s.buildEmbeddingService(ctx, input, asset, resolvedProvider, resolvedModel)
	if err != nil {
		return nil, err
	}
	if embeddingSvc == nil {
		return nil, ErrEmbeddingServiceRequired
	}

	texts := make([]string, 0, len(leafChunks))
	for _, chunk := range leafChunks {
		texts = append(texts, chunk.Content)
	}
	vectors, err := embedDocumentChunkTexts(ctx, embeddingSvc, texts)
	if err != nil {
		return nil, fmt.Errorf("embed document chunks: %w", err)
	}
	if len(vectors) != len(leafChunks) {
		return nil, fmt.Errorf("embedding result count mismatch: got %d, want %d", len(vectors), len(leafChunks))
	}

	items := make([]*model.DocumentChunkEmbedding, 0, len(leafChunks))
	dimension := 0
	for index, chunk := range leafChunks {
		vector := float64ToFloat32Array(vectors[index])
		if len(vector) == 0 {
			return nil, fmt.Errorf("empty embedding vector for chunk %s", chunk.ID)
		}
		if dimension == 0 {
			dimension = len(vector)
		}
		item := &model.DocumentChunkEmbedding{
			ID:                 uuid.New(),
			OrganizationID:     input.OrganizationID,
			WorkspaceID:        asset.WorkspaceID,
			AssetID:            input.AssetID,
			ChunkID:            chunk.ID,
			ProcessingRunID:    input.ProcessingRunID,
			GenerationNo:       input.GenerationNo,
			EmbeddingProvider:  resolvedProvider,
			EmbeddingModel:     resolvedModel,
			EmbeddingDimension: len(vector),
			EmbeddingVector:    vector,
			ContentHash:        chunk.ContentHash,
			Status:             model.DocumentChunkEmbeddingStatusReady,
			MetadataJSON: map[string]any{
				"chunk_type":   chunk.ChunkType,
				"content_hash": chunk.ContentHash,
				"text_hash":    documentEmbeddingContentHash(chunk.Content),
			},
		}
		if err := s.embeddings.Upsert(ctx, item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if s.vectorIndex != nil && indexFileVector && shouldIndexDocumentChunkEmbeddingsForFileQA(clearExisting, asset, resolvedProvider, resolvedModel) {
		if err := s.vectorIndex.IndexChunkEmbeddings(ctx, asset, leafChunks, items, clearExisting); err != nil {
			return nil, fmt.Errorf("index document chunk vectors: %w", err)
		}
	}

	return &GenerateDocumentChunkEmbeddingsResult{
		Embeddings:         items,
		EmbeddingCount:     len(items),
		EmbeddingProvider:  resolvedProvider,
		EmbeddingModel:     resolvedModel,
		EmbeddingDimension: dimension,
	}, nil
}

func (s *documentChunkEmbeddingService) resolveEmbeddingModel(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (string, string, error) {
	provider := strings.TrimSpace(input.EmbeddingProvider)
	modelName := strings.TrimSpace(input.EmbeddingModel)
	if modelName != "" {
		return provider, modelName, nil
	}
	resolved, err := llmruntime.NewModelResolver(s.defaultModelSvc).Resolve(
		ctx,
		input.OrganizationID,
		provider,
		modelName,
		sharedmodel.ModelTypeEmbedding,
	)
	if err != nil {
		return "", "", fmt.Errorf("resolve embedding model: %w", err)
	}
	return resolved.Provider, resolved.Model, nil
}

func (s *documentChunkEmbeddingService) buildEmbeddingService(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error) {
	if s.embeddingFactory != nil {
		return s.embeddingFactory(ctx, input, asset, provider, modelName)
	}
	if s.llmClient == nil {
		return nil, ErrEmbeddingServiceRequired
	}
	workspaceID := input.OrganizationID
	if asset.WorkspaceID != nil && strings.TrimSpace(*asset.WorkspaceID) != "" {
		workspaceID = strings.TrimSpace(*asset.WorkspaceID)
	}
	accountID := strings.TrimSpace(input.RequestedBy)
	if accountID == "" {
		accountID = asset.CreatedBy
	}
	if accountID == "" {
		accountID = input.OrganizationID
	}
	return datasetindexing.NewGatewayEmbeddingService(
		s.llmClient,
		accountID,
		asset.ID.String(),
		"data_library_file",
		modelName,
		workspaceID,
	)
}

func leafDocumentChunks(chunks []*model.DocumentChunk) []*model.DocumentChunk {
	out := make([]*model.DocumentChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil || !chunk.Enabled || chunk.Status != model.DocumentChunkStatusReady {
			continue
		}
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		switch chunk.ChunkType {
		case model.DocumentChunkTypeChild:
			out = append(out, chunk)
		}
	}
	return out
}

func shouldIndexDocumentChunkEmbeddingsForFileQA(clearExisting bool, asset *model.DocumentAsset, provider string, modelName string) bool {
	if clearExisting {
		return true
	}
	if asset == nil || asset.EmbeddingModel == nil {
		return false
	}
	if strings.TrimSpace(*asset.EmbeddingModel) != strings.TrimSpace(modelName) {
		return false
	}
	if asset.EmbeddingProvider == nil {
		return strings.TrimSpace(provider) == ""
	}
	return strings.TrimSpace(*asset.EmbeddingProvider) == strings.TrimSpace(provider)
}

func embedDocumentChunkTexts(ctx context.Context, embeddingSvc embedding.EmbeddingService, texts []string) ([][]float64, error) {
	vectors := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += documentChunkEmbeddingBatchSize {
		end := start + documentChunkEmbeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchVectors, err := embeddingSvc.EmbedTexts(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batchVectors...)
	}
	return vectors, nil
}

func float64ToFloat32Array(values []float64) model.Float32Array {
	out := make(model.Float32Array, len(values))
	for i, value := range values {
		out[i] = float32(value)
	}
	return out
}

func documentEmbeddingContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
