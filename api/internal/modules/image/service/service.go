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
	maxPromptRunes      = 4000
	imageRuntimeAppID   = "image-runtime"
	imageRuntimeAppType = "image-runtime"
	successMessage      = "已生成图片"
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

type generationConversation struct {
	ID           uuid.UUID
	Title        string
	Existing     *model.Conversation
	ShouldCreate bool
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
			continue
		}
		ok, err := s.hasSingleRoute(ctx, scope.OrganizationID, item.Model)
		if err != nil {
			return nil, err
		}
		if ok {
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
	conversation, err := s.resolveGenerationConversation(ctx, chatScope, strings.TrimSpace(req.ConversationID), prompt)
	if err != nil {
		return nil, ErrConversationNotAccessible
	}
	conversationID := conversation.ID.String()
	appCtx, err := buildAppContext(scope, conversation.ID)
	if err != nil {
		return nil, err
	}
	n := count
	imageReq := &adapter.ImageRequest{
		Model:          modelSpec.Model,
		Prompt:         prompt,
		N:              &n,
		Size:           size,
		ResponseFormat: imageResponseFormat(modelSpec),
		User:           scope.AccountID.String(),
	}
	resp, err := s.llmClient.AppCreateImage(ctx, appCtx, imageReq)
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
			if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
				return nil, fmt.Errorf("%w: %v; cleanup failed: %v", ErrImageSaveFailed, err, cleanupErr)
			}
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
	messageReq := chatruntime.CreateCompletedMessageRequest{
		ConversationID: conversation.ID,
		Query:          prompt,
		Answer:         successMessage,
		ModelProvider:  modelSpec.Provider,
		ModelName:      modelSpec.Model,
		Metadata:       map[string]interface{}{"image_generation": generation},
	}
	var completed *model.Message
	if conversation.ShouldCreate {
		createdConversation, message, createErr := s.chatService.CreateConversationWithCompletedMessage(ctx, chatScope, chatruntime.Caller{
			Type:             model.ConversationCallerAIChat,
			ConversationType: model.ConversationTypeImage,
		}, chatruntime.CreateConversationWithCompletedMessageRequest{
			ConversationID: conversation.ID,
			Title:          conversation.Title,
			Message:        messageReq,
		})
		if createErr != nil {
			if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
				return nil, fmt.Errorf("%w; cleanup failed: %v", createErr, cleanupErr)
			}
			return nil, createErr
		}
		conversation.Existing = createdConversation
		completed = message
	} else {
		message, createErr := s.chatService.CreateCompletedMessage(ctx, chatScope, messageReq)
		if createErr != nil {
			if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
				return nil, fmt.Errorf("%w; cleanup failed: %v", createErr, cleanupErr)
			}
			return nil, createErr
		}
		completed = message
	}
	if completed == nil {
		if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
			return nil, fmt.Errorf("image message was not created; cleanup failed: %w", cleanupErr)
		}
		return nil, fmt.Errorf("image message was not created")
	}
	if conversation.Existing == nil {
		if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
			return nil, fmt.Errorf("image conversation was not created; cleanup failed: %w", cleanupErr)
		}
		return nil, fmt.Errorf("image conversation was not created")
	}
	return &GenerateResult{
		ConversationID:  conversation.Existing.ID.String(),
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
	ok, err := s.hasSingleRoute(ctx, organizationID, modelSpec.Model)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return ErrModelNotAvailable
}

func (s *service) hasSingleRoute(ctx context.Context, organizationID uuid.UUID, modelName string) (bool, error) {
	if s.routes == nil {
		return false, nil
	}
	routes, err := s.routes.GetRoutesForModel(ctx, organizationID, modelName)
	if err != nil {
		return false, err
	}
	if len(routes) > 1 {
		return false, ErrModelRouteAmbiguous
	}
	return len(routes) == 1, nil
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

func (s *service) resolveGenerationConversation(ctx context.Context, scope chatruntime.Scope, rawID string, prompt string) (*generationConversation, error) {
	if rawID == "" {
		title := prompt
		if len([]rune(title)) > 50 {
			title = string([]rune(title)[:50])
		}
		return &generationConversation{
			ID:           uuid.New(),
			Title:        title,
			ShouldCreate: true,
		}, nil
	}
	conversationID, err := uuid.Parse(rawID)
	if err != nil {
		return nil, err
	}
	conversation, err := s.chatService.GetConversationByCaller(ctx, scope, chatruntime.Caller{
		Type:             model.ConversationCallerAIChat,
		ConversationType: model.ConversationTypeImage,
	}, conversationID)
	if err != nil {
		return nil, err
	}
	return &generationConversation{
		ID:       conversation.ID,
		Title:    conversation.Title,
		Existing: conversation,
	}, nil
}

func (s *service) cleanupGeneratedFiles(ctx context.Context, files []ImageFile) error {
	var cleanupErr error
	for _, file := range files {
		fileID := strings.TrimSpace(file.FileID)
		if fileID == "" {
			continue
		}
		if err := s.imageAssets.DeleteGeneratedImage(ctx, fileID); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete generated image %s: %w", fileID, err))
		}
	}
	return cleanupErr
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

func imageResponseFormat(modelSpec registry.ImageModel) string {
	if modelSpec.Provider == "openai" && strings.HasPrefix(modelSpec.Model, "gpt-image") {
		return ""
	}
	return "url"
}

func buildAppContext(scope Scope, conversationID uuid.UUID) (*llmclient.AppContext, error) {
	if scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil || conversationID == uuid.Nil {
		return nil, ErrBillingContextRequired
	}
	if scope.WorkspaceID == nil || *scope.WorkspaceID == uuid.Nil {
		return nil, ErrBillingContextRequired
	}
	sessionID := conversationID.String()
	return &llmclient.AppContext{
		OrganizationID:     scope.OrganizationID.String(),
		WorkspaceID:        scope.WorkspaceID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace,
		AppID:              imageRuntimeAppID,
		AppType:            imageRuntimeAppType,
		AccountID:          scope.AccountID.String(),
		SessionID:          sessionID,
		ConversationID:     sessionID,
	}, nil
}

func imageFileFromMeta(meta map[string]interface{}) ImageFile {
	return ImageFile{
		FileID:         stringValue(meta["file_id"]),
		ToolFileID:     stringValue(meta["tool_file_id"]),
		URL:            stringValue(meta["url"]),
		DownloadURL:    stringValue(meta["download_url"]),
		Filename:       stringValue(meta["filename"]),
		Extension:      stringValue(meta["extension"]),
		MimeType:       stringValue(meta["mime_type"]),
		TransferMethod: stringValue(meta["transfer_method"]),
		Lifecycle:      stringValue(meta["lifecycle"]),
		ExpiresAt:      int64PtrValue(meta["expires_at"]),
	}
}

func int64PtrValue(value interface{}) *int64 {
	switch typed := value.(type) {
	case int:
		out := int64(typed)
		return &out
	case int64:
		out := typed
		return &out
	case float64:
		out := int64(typed)
		return &out
	default:
		return nil
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
		ErrBillingContextRequired,
		ErrUpstreamFailed,
		ErrImageSaveFailed,
	} {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}
	return "IMAGE_RUNTIME_FAILED"
}
