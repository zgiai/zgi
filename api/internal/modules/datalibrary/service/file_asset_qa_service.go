package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
)

var (
	ErrFileAssetQAQuestionRequired = errors.New("question is required")
	ErrFileAssetQAIndexNotReady    = errors.New("document qa index is not ready")
)

type FileAssetQAService interface {
	AskCurrentFile(ctx context.Context, input FileAssetQAInput) (*FileAssetQAResult, error)
	StreamCurrentFile(ctx context.Context, input FileAssetQAInput) (<-chan FileAssetQAStreamEvent, error)
}

type FileAssetQAInput struct {
	OrganizationID string
	SourceFileID   string
	Question       string
	TopK           int
	AccountID      string
}

type FileAssetQAResult struct {
	Answer    string               `json:"answer"`
	Sources   []*FileAssetQASource `json:"sources"`
	Retrieval FileAssetQARetrieval `json:"retrieval"`
}

type FileAssetQAStreamEvent struct {
	Type      string                `json:"type"`
	Delta     string                `json:"delta,omitempty"`
	Answer    string                `json:"answer,omitempty"`
	Sources   []*FileAssetQASource  `json:"sources,omitempty"`
	Retrieval *FileAssetQARetrieval `json:"retrieval,omitempty"`
	Error     string                `json:"error,omitempty"`
}

type FileAssetQARetrieval struct {
	TopK              int    `json:"top_k"`
	HitCount          int    `json:"hit_count"`
	PrimaryHitCount   int    `json:"primary_hit_count"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"`
	EmbeddingModel    string `json:"embedding_model,omitempty"`
	AnswerModel       string `json:"answer_model,omitempty"`
}

type FileAssetQASource struct {
	PrimaryChunkID string                   `json:"primary_chunk_id"`
	Position       int                      `json:"position"`
	Content        string                   `json:"content"`
	Snippet        string                   `json:"snippet"`
	Score          *float64                 `json:"score,omitempty"`
	Distance       *float64                 `json:"distance,omitempty"`
	Children       []*FileAssetQAChildMatch `json:"children"`
}

type FileAssetQAChildMatch struct {
	ChunkID  string   `json:"chunk_id"`
	Position int      `json:"position"`
	Content  string   `json:"content"`
	Snippet  string   `json:"snippet"`
	Score    *float64 `json:"score,omitempty"`
	Distance *float64 `json:"distance,omitempty"`
}

type fileAssetQAService struct {
	assets          repository.DocumentAssetRepository
	chunks          repository.DocumentChunkRepository
	embeddings      repository.DocumentChunkEmbeddingRepository
	vectorIndex     FileAssetVectorIndexService
	llmClient       llmclient.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
}

type preparedFileAssetQA struct {
	Asset     *model.DocumentAsset
	Question  string
	Sources   []*FileAssetQASource
	Retrieval FileAssetQARetrieval
	AccountID string
}

func NewFileAssetQAService(
	assets repository.DocumentAssetRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
	vectorIndex FileAssetVectorIndexService,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) FileAssetQAService {
	return &fileAssetQAService{
		assets:          assets,
		chunks:          chunks,
		embeddings:      embeddings,
		vectorIndex:     vectorIndex,
		llmClient:       llmClient,
		defaultModelSvc: defaultModelSvc,
	}
}

func (s *fileAssetQAService) AskCurrentFile(ctx context.Context, input FileAssetQAInput) (*FileAssetQAResult, error) {
	prepared, err := s.prepareCurrentFileQA(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(prepared.Sources) == 0 {
		return &FileAssetQAResult{
			Answer:    "未在文档中找到相关信息。",
			Sources:   []*FileAssetQASource{},
			Retrieval: prepared.Retrieval,
		}, nil
	}
	answerModel, answer, err := s.generateAnswer(ctx, prepared.Asset, prepared.Question, prepared.Sources, prepared.AccountID)
	if err != nil {
		return nil, err
	}
	prepared.Retrieval.AnswerModel = answerModel
	return &FileAssetQAResult{
		Answer:    answer,
		Sources:   prepared.Sources,
		Retrieval: prepared.Retrieval,
	}, nil
}

func (s *fileAssetQAService) StreamCurrentFile(ctx context.Context, input FileAssetQAInput) (<-chan FileAssetQAStreamEvent, error) {
	prepared, err := s.prepareCurrentFileQA(ctx, input)
	if err != nil {
		return nil, err
	}

	answerModel := ""
	var req *llmadapter.ChatRequest
	if len(prepared.Sources) > 0 {
		answerModel, req, err = s.buildAnswerRequest(ctx, prepared.Asset, prepared.Question, prepared.Sources, prepared.AccountID)
		if err != nil {
			return nil, err
		}
		prepared.Retrieval.AnswerModel = answerModel
	}

	out := make(chan FileAssetQAStreamEvent, 16)
	go func() {
		defer close(out)
		out <- FileAssetQAStreamEvent{
			Type:      "retrieval",
			Sources:   prepared.Sources,
			Retrieval: &prepared.Retrieval,
		}

		if len(prepared.Sources) == 0 {
			out <- FileAssetQAStreamEvent{
				Type:      "done",
				Answer:    "未在文档中找到相关信息。",
				Sources:   []*FileAssetQASource{},
				Retrieval: &prepared.Retrieval,
			}
			return
		}

		stream, streamErr := s.llmClient.AppChatStream(ctx, qaAppContext(prepared.Asset, qaAccountID(prepared.Asset, prepared.AccountID)), req)
		if streamErr != nil {
			out <- FileAssetQAStreamEvent{Type: "error", Error: fmt.Sprintf("generate qa answer stream: %v", streamErr)}
			return
		}

		var answer strings.Builder
		for event := range stream {
			if event.Error != nil {
				out <- FileAssetQAStreamEvent{Type: "error", Error: event.Error.Error()}
				return
			}
			for _, choice := range event.Choices {
				delta := extractStreamDelta(choice.Delta.Content)
				if delta == "" {
					continue
				}
				answer.WriteString(delta)
				out <- FileAssetQAStreamEvent{Type: "delta", Delta: delta}
			}
			if event.Done {
				break
			}
		}
		finalAnswer := strings.TrimSpace(answer.String())
		if finalAnswer == "" {
			finalAnswer = "未在文档中找到相关信息。"
		}
		out <- FileAssetQAStreamEvent{
			Type:      "done",
			Answer:    finalAnswer,
			Sources:   prepared.Sources,
			Retrieval: &prepared.Retrieval,
		}
	}()
	return out, nil
}

func (s *fileAssetQAService) prepareCurrentFileQA(ctx context.Context, input FileAssetQAInput) (*preparedFileAssetQA, error) {
	if strings.TrimSpace(input.OrganizationID) == "" {
		return nil, ErrOrganizationIDRequired
	}
	if strings.TrimSpace(input.SourceFileID) == "" {
		return nil, ErrSourceFileIDRequired
	}
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return nil, ErrFileAssetQAQuestionRequired
	}
	if s == nil || s.assets == nil || s.chunks == nil || s.embeddings == nil || s.vectorIndex == nil || s.llmClient == nil {
		return nil, ErrEmbeddingServiceRequired
	}
	asset, err := s.assets.FindAssetBySourceFileID(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, ErrDocumentAssetNotFound
	}
	if !isFileAssetQAReady(asset) {
		return nil, ErrFileAssetQAIndexNotReady
	}
	embeddingCount, err := s.embeddings.CountReadyByAssetGeneration(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	if embeddingCount <= 0 {
		return nil, ErrFileAssetQAIndexNotReady
	}
	if err := s.vectorIndex.EnsureAssetIndexed(ctx, asset); err != nil {
		return nil, fmt.Errorf("ensure file vector index: %w", err)
	}
	topK := normalizeFileQATopK(input.TopK)
	embeddingProvider, embeddingModel, err := s.resolveEmbeddingModel(ctx, asset)
	if err != nil {
		return nil, err
	}
	queryVector, err := s.embedQuestion(ctx, asset, question, embeddingModel, input.AccountID)
	if err != nil {
		return nil, err
	}
	rawHits, err := s.vectorIndex.Search(ctx, asset, queryVector, topK*4)
	if err != nil {
		return nil, err
	}
	sources, err := s.buildSources(ctx, asset, rawHits, topK)
	if err != nil {
		return nil, err
	}
	retrieval := FileAssetQARetrieval{
		TopK:              topK,
		HitCount:          len(rawHits),
		PrimaryHitCount:   len(sources),
		EmbeddingProvider: embeddingProvider,
		EmbeddingModel:    embeddingModel,
	}
	return &preparedFileAssetQA{
		Asset:     asset,
		Question:  question,
		Sources:   sources,
		Retrieval: retrieval,
		AccountID: input.AccountID,
	}, nil
}

func (s *fileAssetQAService) resolveEmbeddingModel(ctx context.Context, asset *model.DocumentAsset) (string, string, error) {
	if asset.EmbeddingModel != nil && strings.TrimSpace(*asset.EmbeddingModel) != "" {
		provider := ""
		if asset.EmbeddingProvider != nil {
			provider = strings.TrimSpace(*asset.EmbeddingProvider)
		}
		return provider, strings.TrimSpace(*asset.EmbeddingModel), nil
	}
	items, _, err := s.embeddings.List(ctx, repository.DocumentChunkEmbeddingListFilter{
		OrganizationID: asset.OrganizationID,
		AssetID:        asset.ID,
		GenerationNo:   &asset.GenerationNo,
		Status:         model.DocumentChunkEmbeddingStatusReady,
		Limit:          1,
		Offset:         0,
	})
	if err != nil {
		return "", "", err
	}
	if len(items) > 0 && items[0] != nil && strings.TrimSpace(items[0].EmbeddingModel) != "" {
		return strings.TrimSpace(items[0].EmbeddingProvider), strings.TrimSpace(items[0].EmbeddingModel), nil
	}
	resolved, err := llmruntime.NewModelResolver(s.defaultModelSvc).ResolveDefault(ctx, asset.OrganizationID, sharedmodel.ModelTypeEmbedding)
	if err != nil {
		return "", "", fmt.Errorf("resolve qa embedding model: %w", err)
	}
	return resolved.Provider, resolved.Model, nil
}

func (s *fileAssetQAService) embedQuestion(ctx context.Context, asset *model.DocumentAsset, question string, modelName string, accountID string) ([]float64, error) {
	accountID = qaAccountID(asset, accountID)
	req := &llmadapter.EmbeddingsRequest{
		Input: []string{question},
		Model: modelName,
		User:  accountID,
	}
	resp, err := s.llmClient.AppEmbed(ctx, qaAppContext(asset, accountID), req)
	if err != nil {
		return nil, fmt.Errorf("embed qa question: %w", err)
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, ErrDocumentChunkEmbeddingsRequired
	}
	return adapterFloat32To64(resp.Data[0].Embedding), nil
}

func (s *fileAssetQAService) buildSources(ctx context.Context, asset *model.DocumentAsset, rawHits []map[string]interface{}, topK int) ([]*FileAssetQASource, error) {
	childIDs := make([]uuid.UUID, 0, len(rawHits))
	hitsByChild := make(map[uuid.UUID]fileQAHit, len(rawHits))
	for order, raw := range rawHits {
		childID, err := parseFileQAHitChunkID(raw)
		if err != nil || childID == uuid.Nil {
			continue
		}
		if _, exists := hitsByChild[childID]; exists {
			continue
		}
		hit := fileQAHit{Order: order}
		if distance, ok := parseFileQAHitDistance(raw); ok {
			hit.Distance = &distance
			score := 1 - distance
			hit.Score = &score
		}
		hitsByChild[childID] = hit
		childIDs = append(childIDs, childID)
	}
	children, err := s.chunks.ListByIDs(ctx, asset.OrganizationID, childIDs)
	if err != nil {
		return nil, err
	}
	parentIDs := make([]uuid.UUID, 0, len(children))
	childByID := make(map[uuid.UUID]*model.DocumentChunk, len(children))
	for _, child := range children {
		if !isCurrentReadyChunk(asset, child, model.DocumentChunkTypeChild) || child.ParentChunkID == nil {
			continue
		}
		childByID[child.ID] = child
		parentIDs = append(parentIDs, *child.ParentChunkID)
	}
	parents, err := s.chunks.ListByIDs(ctx, asset.OrganizationID, parentIDs)
	if err != nil {
		return nil, err
	}
	parentByID := make(map[uuid.UUID]*model.DocumentChunk, len(parents))
	for _, parent := range parents {
		if isCurrentReadyChunk(asset, parent, model.DocumentChunkTypeParent) {
			parentByID[parent.ID] = parent
		}
	}
	byParent := make(map[uuid.UUID]*FileAssetQASource)
	for _, childID := range childIDs {
		child := childByID[childID]
		if child == nil || child.ParentChunkID == nil {
			continue
		}
		parent := parentByID[*child.ParentChunkID]
		if parent == nil {
			continue
		}
		hit := hitsByChild[childID]
		source := byParent[parent.ID]
		if source == nil {
			source = &FileAssetQASource{
				PrimaryChunkID: parent.ID.String(),
				Position:       parent.Position,
				Content:        parent.Content,
				Snippet:        truncateRunes(parent.Content, 320),
				Score:          hit.Score,
				Distance:       hit.Distance,
				Children:       []*FileAssetQAChildMatch{},
			}
			byParent[parent.ID] = source
		} else if isBetterFileQAHit(hit, source.Distance) {
			source.Score = hit.Score
			source.Distance = hit.Distance
		}
		source.Children = append(source.Children, &FileAssetQAChildMatch{
			ChunkID:  child.ID.String(),
			Position: child.Position,
			Content:  child.Content,
			Snippet:  truncateRunes(child.Content, 240),
			Score:    hit.Score,
			Distance: hit.Distance,
		})
	}
	sources := make([]*FileAssetQASource, 0, len(byParent))
	for _, source := range byParent {
		sources = append(sources, source)
	}
	sort.SliceStable(sources, func(i, j int) bool {
		left, right := sources[i], sources[j]
		if left.Distance != nil && right.Distance != nil && *left.Distance != *right.Distance {
			return *left.Distance < *right.Distance
		}
		return left.Position < right.Position
	})
	if len(sources) > topK {
		sources = sources[:topK]
	}
	return sources, nil
}

func (s *fileAssetQAService) generateAnswer(ctx context.Context, asset *model.DocumentAsset, question string, sources []*FileAssetQASource, accountID string) (string, string, error) {
	answerModel, req, err := s.buildAnswerRequest(ctx, asset, question, sources, accountID)
	if err != nil {
		return answerModel, "", err
	}
	resp, err := s.llmClient.AppChat(ctx, qaAppContext(asset, qaAccountID(asset, accountID)), req)
	if err != nil {
		return answerModel, "", fmt.Errorf("generate qa answer: %w", err)
	}
	answer := extractChatAnswer(resp)
	if answer == "" {
		answer = "未在文档中找到相关信息。"
	}
	return answerModel, answer, nil
}

func (s *fileAssetQAService) buildAnswerRequest(ctx context.Context, asset *model.DocumentAsset, question string, sources []*FileAssetQASource, accountID string) (string, *llmadapter.ChatRequest, error) {
	if s.defaultModelSvc == nil {
		return "", nil, ErrEmbeddingServiceRequired
	}
	resolved, err := llmruntime.NewModelResolver(s.defaultModelSvc).ResolveDefault(ctx, asset.OrganizationID, sharedmodel.ModelTypeLLM)
	if err != nil {
		return "", nil, fmt.Errorf("resolve qa answer model: %w", err)
	}
	temperature := 0.2
	maxTokens := 900
	req := &llmadapter.ChatRequest{
		Provider:    resolved.Provider,
		Model:       resolved.Model,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Messages: []llmadapter.Message{
			{
				Role:    "system",
				Content: "你是文档问答助手。只能依据提供的文档片段回答问题；如果片段中没有依据，回答“未在文档中找到相关信息”。回答应简洁，只输出答案正文，不要在回答末尾附加“依据来源”、切片编号或引用列表。",
			},
			{
				Role:    "user",
				Content: buildFileQAUserPrompt(question, sources),
			},
		},
		User: qaAccountID(asset, accountID),
	}
	return resolved.Model, req, nil
}

type fileQAHit struct {
	Order    int
	Score    *float64
	Distance *float64
}

func isFileAssetQAReady(asset *model.DocumentAsset) bool {
	return asset != nil &&
		asset.ProductStatus == model.DocumentAssetProductStatusReady &&
		asset.VectorStatus == model.DocumentAssetVectorStatusReady &&
		asset.GenerationNo > 0
}

func normalizeFileQATopK(value int) int {
	if value <= 0 {
		return 6
	}
	if value > 20 {
		return 20
	}
	return value
}

func isCurrentReadyChunk(asset *model.DocumentAsset, chunk *model.DocumentChunk, chunkType string) bool {
	return asset != nil &&
		chunk != nil &&
		chunk.OrganizationID == asset.OrganizationID &&
		chunk.AssetID == asset.ID &&
		chunk.GenerationNo == asset.GenerationNo &&
		chunk.ChunkType == chunkType &&
		chunk.Enabled &&
		chunk.Status == model.DocumentChunkStatusReady
}

func parseFileQAHitChunkID(raw map[string]interface{}) (uuid.UUID, error) {
	if value, ok := raw["doc_id"].(string); ok && value != "" {
		return uuid.Parse(value)
	}
	if additional, ok := raw["_additional"].(map[string]interface{}); ok {
		if value, ok := additional["id"].(string); ok && value != "" {
			return uuid.Parse(value)
		}
	}
	return uuid.Nil, errors.New("chunk id missing")
}

func parseFileQAHitDistance(raw map[string]interface{}) (float64, bool) {
	additional, ok := raw["_additional"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	switch value := additional["distance"].(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

func isBetterFileQAHit(hit fileQAHit, current *float64) bool {
	if hit.Distance == nil {
		return current == nil
	}
	if current == nil {
		return true
	}
	return *hit.Distance < *current
}

func buildFileQAUserPrompt(question string, sources []*FileAssetQASource) string {
	var b strings.Builder
	b.WriteString("问题：\n")
	b.WriteString(question)
	b.WriteString("\n\n文档片段：\n")
	for i, source := range sources {
		b.WriteString(fmt.Sprintf("[一级切片 %d / #%d]\n", i+1, source.Position+1))
		b.WriteString(truncateRunes(source.Content, 1600))
		b.WriteString("\n")
		for j, child := range source.Children {
			b.WriteString(fmt.Sprintf("[二级切片 %d.%d / #S-%d]\n", i+1, j+1, child.Position+1))
			b.WriteString(truncateRunes(child.Content, 500))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func qaAccountID(asset *model.DocumentAsset, accountID string) string {
	if strings.TrimSpace(accountID) != "" {
		return strings.TrimSpace(accountID)
	}
	if asset != nil && strings.TrimSpace(asset.CreatedBy) != "" {
		return strings.TrimSpace(asset.CreatedBy)
	}
	if asset != nil {
		return asset.OrganizationID
	}
	return ""
}

func qaAppContext(asset *model.DocumentAsset, accountID string) *llmclient.AppContext {
	workspaceID := ""
	if asset != nil && asset.WorkspaceID != nil {
		workspaceID = strings.TrimSpace(*asset.WorkspaceID)
	}
	if workspaceID == "" && asset != nil {
		workspaceID = asset.OrganizationID
	}
	return &llmclient.AppContext{
		OrganizationID: asset.OrganizationID,
		WorkspaceID:    workspaceID,
		AppID:          asset.ID.String(),
		AppType:        "data_library_file",
		AccountID:      accountID,
	}
}

func extractChatAnswer(resp *llmadapter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	switch content := resp.Choices[0].Message.Content.(type) {
	case string:
		return strings.TrimSpace(content)
	case []llmadapter.MessageContentPart:
		parts := make([]string, 0, len(content))
		for _, part := range content {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func extractStreamDelta(content interface{}) string {
	switch value := content.(type) {
	case string:
		return value
	case []llmadapter.MessageContentPart:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func adapterFloat32To64(values []float32) []float64 {
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = float64(value)
	}
	return out
}
