package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/imageasset"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	maxPromptRunes = 4000
	successMessage = "已生成图片"
)

type Service interface {
	ListModels(ctx context.Context, scope Scope) ([]registry.ImageModel, error)
	Generate(ctx context.Context, scope Scope, req GenerateRequest) (*GenerateResult, error)
}

type RouteLister interface {
	GetRoutesForModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*channelmodel.RouteQueryResult, error)
}

type service struct {
	registry        *registry.Registry
	availableModels llmmodelsvc.AvailableModelsService
	routes          RouteLister
	llmClient       llmclient.LLMClient
	chatService     chatruntime.Service
	imageAssets     imageasset.Service
}

func NewService(reg *registry.Registry, availableModels llmmodelsvc.AvailableModelsService, routes RouteLister, llmClient llmclient.LLMClient, chatService chatruntime.Service, imageAssets imageasset.Service) Service {
	return &service{
		registry:        reg,
		availableModels: availableModels,
		routes:          routes,
		llmClient:       llmClient,
		chatService:     chatService,
		imageAssets:     imageAssets,
	}
}

func (s *service) ListModels(ctx context.Context, scope Scope) ([]registry.ImageModel, error) {
	available, err := s.availableImageModels(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	if hasAmbiguousModel(available) {
		return nil, ErrModelRouteAmbiguous
	}
	byKey := map[string]struct{}{}
	for _, item := range available {
		byKey[modelKey(item.Provider, item.Name)] = struct{}{}
	}
	result := make([]registry.ImageModel, 0)
	for _, item := range s.registry.ListEnabled() {
		if _, ok := byKey[modelKey(item.Provider, item.Model)]; ok {
			if err := s.ensureSingleRoute(ctx, scope.OrganizationID, item.Model); err != nil {
				return nil, err
			}
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *service) Generate(ctx context.Context, scope Scope, req GenerateRequest) (*GenerateResult, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, ErrPromptRequired
	}
	if len([]rune(prompt)) > maxPromptRunes {
		return nil, ErrPromptTooLong
	}
	modelSpec, ok := s.registry.Get(req.Provider, req.Model)
	if !ok {
		return nil, ErrModelNotAvailable
	}
	size := strings.TrimSpace(req.Size)
	if size == "" {
		size = modelSpec.DefaultSize
	}
	if !containsString(modelSpec.SupportedSizes, size) {
		return nil, ErrUnsupportedSize
	}
	count := req.Count
	if count == 0 {
		count = modelSpec.DefaultCount
	}
	if !containsInt(modelSpec.SupportedCounts, count) {
		return nil, ErrUnsupportedCount
	}
	if err := s.ensureModelAvailable(ctx, scope.OrganizationID, modelSpec); err != nil {
		return nil, err
	}

	chatScope := chatruntime.Scope{
		OrganizationID: scope.OrganizationID,
		AccountID:      scope.AccountID,
		WorkspaceID:    scope.WorkspaceID,
	}
	conversation, err := s.resolveConversation(ctx, chatScope, strings.TrimSpace(req.ConversationID), prompt)
	if err != nil {
		return nil, ErrConversationNotAccessible
	}
	conversationID := conversation.ID.String()
	n := count
	imageReq := &adapter.ImageRequest{
		Model:          modelSpec.Model,
		Prompt:         prompt,
		N:              &n,
		Size:           size,
		ResponseFormat: "url",
		User:           scope.AccountID.String(),
	}
	resp, err := s.llmClient.CreateImage(ctx, scope.OrganizationID.String(), imageReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, ErrUpstreamFailed
	}

	files := make([]ImageFile, 0, len(resp.Data))
	conversationIDPtr := conversationID
	for idx, item := range resp.Data {
		fileMeta, err := s.imageAssets.SaveGeneratedImage(ctx, imageasset.SaveRequest{
			TenantID:       scope.OrganizationID.String(),
			UserID:         scope.AccountID.String(),
			ConversationID: &conversationIDPtr,
			Item:           item,
			BaseFilename:   "generated-image",
			Index:          idx,
			Lifecycle:      tool_file.ToolFileLifecyclePersistent,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrImageSaveFailed, err)
		}
		files = append(files, imageFileFromMeta(fileMeta))
	}

	generation := ImageGenerationMetadata{
		Provider:   modelSpec.Provider,
		Model:      modelSpec.Model,
		ModelLabel: modelSpec.ModelLabel,
		Size:       size,
		Count:      count,
		Files:      files,
		Status:     "succeeded",
	}
	completed, err := s.chatService.CreateCompletedMessage(ctx, chatScope, chatruntime.CreateCompletedMessageRequest{
		ConversationID: conversation.ID,
		Query:          prompt,
		Answer:         successMessage,
		ModelProvider:  modelSpec.Provider,
		ModelName:      modelSpec.Model,
		Metadata:       map[string]interface{}{"image_generation": generation},
	})
	if err != nil {
		return nil, err
	}
	return &GenerateResult{
		ConversationID:  conversation.ID.String(),
		MessageID:       completed.ID.String(),
		Message:         successMessage,
		ImageGeneration: generation,
	}, nil
}

func (s *service) availableImageModels(ctx context.Context, organizationID uuid.UUID) ([]*llmmodelsvc.AvailableModel, error) {
	if s.availableModels == nil {
		return nil, ErrModelNotAvailable
	}
	return s.availableModels.ListAvailable(ctx, organizationID, "", string(llmmodelmodel.UseCaseImageGen))
}

func (s *service) ensureModelAvailable(ctx context.Context, organizationID uuid.UUID, modelSpec registry.ImageModel) error {
	available, err := s.availableImageModels(ctx, organizationID)
	if err != nil {
		return err
	}
	if hasAmbiguousModel(available) {
		return ErrModelRouteAmbiguous
	}
	for _, item := range available {
		if item.Provider == modelSpec.Provider && item.Name == modelSpec.Model {
			return s.ensureSingleRoute(ctx, organizationID, modelSpec.Model)
		}
	}
	return ErrModelNotAvailable
}

func (s *service) ensureSingleRoute(ctx context.Context, organizationID uuid.UUID, modelName string) error {
	if s.routes == nil {
		return nil
	}
	routes, err := s.routes.GetRoutesForModel(ctx, organizationID, modelName)
	if err != nil {
		return err
	}
	if len(routes) > 1 {
		return ErrModelRouteAmbiguous
	}
	return nil
}

func (s *service) resolveConversation(ctx context.Context, scope chatruntime.Scope, rawID string, prompt string) (*model.Conversation, error) {
	if rawID == "" {
		title := prompt
		if len([]rune(title)) > 50 {
			title = string([]rune(title)[:50])
		}
		return s.chatService.CreateConversation(ctx, scope, title)
	}
	conversationID, err := uuid.Parse(rawID)
	if err != nil {
		return nil, err
	}
	return s.chatService.GetConversation(ctx, scope, conversationID)
}

func hasAmbiguousModel(models []*llmmodelsvc.AvailableModel) bool {
	seen := map[string]string{}
	for _, item := range models {
		modelName := strings.TrimSpace(item.Name)
		provider := strings.TrimSpace(item.Provider)
		if modelName == "" {
			continue
		}
		if prev, ok := seen[modelName]; ok && prev != provider {
			return true
		}
		seen[modelName] = provider
	}
	return false
}

func modelKey(provider, model string) string {
	return strings.TrimSpace(provider) + "/" + strings.TrimSpace(model)
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func containsInt(items []int, value int) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func imageFileFromMeta(meta map[string]interface{}) ImageFile {
	return ImageFile{
		FileID:      stringValue(meta["file_id"]),
		URL:         stringValue(meta["url"]),
		DownloadURL: stringValue(meta["download_url"]),
		Filename:    stringValue(meta["filename"]),
		Extension:   stringValue(meta["extension"]),
		MimeType:    stringValue(meta["mime_type"]),
	}
}

func stringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func ErrorCode(err error) string {
	for _, candidate := range []error{
		ErrPromptRequired,
		ErrPromptTooLong,
		ErrModelNotAvailable,
		ErrModelRouteAmbiguous,
		ErrUnsupportedSize,
		ErrUnsupportedCount,
		ErrConversationNotAccessible,
		ErrUpstreamFailed,
		ErrImageSaveFailed,
	} {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}
	return "IMAGE_RUNTIME_FAILED"
}
