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
	result := make([]registry.ImageModel, 0, len(available))
	for _, item := range available {
		if item == nil {
			continue
		}
		routes, err := s.routesForModel(ctx, scope.OrganizationID, item.Name)
		if err != nil {
			return nil, err
		}
		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = strings.TrimSpace(item.Name)
		}
		result = append(result, registry.ImageModel{
			Provider:          strings.TrimSpace(item.Provider),
			Model:             strings.TrimSpace(item.Name),
			ModelLabel:        label,
			GenerationProfile: s.registry.Resolve(item.Provider, item.Name, routes),
		})
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
	availableModel, err := s.findAvailableModel(ctx, scope.OrganizationID, req.Provider, req.Model)
	if err != nil {
		return nil, err
	}
	if availableModel == nil {
		return nil, ErrModelNotAvailable
	}
	routes, err := s.routesForModel(ctx, scope.OrganizationID, availableModel.Name)
	if err != nil {
		return nil, err
	}
	modelSpec := registry.ImageModel{
		Provider:   strings.TrimSpace(availableModel.Provider),
		Model:      strings.TrimSpace(availableModel.Name),
		ModelLabel: strings.TrimSpace(availableModel.DisplayName),
	}
	if modelSpec.ModelLabel == "" {
		modelSpec.ModelLabel = modelSpec.Model
	}
	modelSpec.GenerationProfile = s.registry.Resolve(modelSpec.Provider, modelSpec.Model, routes)
	options, err := validateGenerateOptions(modelSpec.GenerationProfile, req.Options)
	if err != nil {
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
	imageReq := &adapter.ImageRequest{
		Provider:       modelSpec.Provider,
		Model:          modelSpec.Model,
		Prompt:         prompt,
		N:              options.Count,
		Size:           options.Size,
		User:           scope.AccountID.String(),
		GenerationMode: options.GenerationMode,
		MaxImages:      options.MaxImages,
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
		Provider:       modelSpec.Provider,
		Model:          modelSpec.Model,
		ModelLabel:     modelSpec.ModelLabel,
		Size:           options.Size,
		Count:          len(files),
		GenerationMode: options.GenerationMode,
		MaxImages:      options.MaxImages,
		Files:          files,
		Status:         "succeeded",
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

func (s *service) findAvailableModel(ctx context.Context, organizationID uuid.UUID, provider, modelName string) (*llmmodelsvc.AvailableModel, error) {
	available, err := s.availableImageModels(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	for _, item := range available {
		if item != nil && strings.TrimSpace(item.Provider) == strings.TrimSpace(provider) && strings.TrimSpace(item.Name) == strings.TrimSpace(modelName) {
			return item, nil
		}
	}
	return nil, nil
}

func (s *service) routesForModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*channelmodel.RouteQueryResult, error) {
	if s.routes == nil {
		return nil, nil
	}
	return s.routes.GetRoutesForModel(ctx, organizationID, modelName)
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
	if conversation.LegacyFallback {
		return nil, ErrConversationNotAccessible
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

func validateGenerateOptions(profile registry.GenerationProfile, requested GenerateOptions) (GenerateOptions, error) {
	options := GenerateOptions{
		Size:           strings.TrimSpace(requested.Size),
		Count:          requested.Count,
		GenerationMode: strings.TrimSpace(requested.GenerationMode),
		MaxImages:      requested.MaxImages,
	}
	if profile.Size == nil {
		if options.Size != "" {
			return GenerateOptions{}, ErrParameterNotSupported
		}
	} else {
		if options.Size == "" {
			options.Size = profile.Size.Default
		}
		if !profileSupportsSize(profile.Size, options.Size) {
			return GenerateOptions{}, ErrUnsupportedSize
		}
	}
	if profile.Quantity == nil {
		if options.Count != nil || options.GenerationMode != "" || options.MaxImages != nil {
			return GenerateOptions{}, ErrParameterNotSupported
		}
		return options, nil
	}
	switch profile.Quantity.Mode {
	case registry.QuantityModeExact:
		if options.GenerationMode != "" || options.MaxImages != nil {
			return GenerateOptions{}, ErrParameterNotSupported
		}
		if options.Count == nil {
			value := profile.Quantity.Default
			options.Count = &value
		}
		if *options.Count < profile.Quantity.Min || *options.Count > profile.Quantity.Max {
			return GenerateOptions{}, ErrUnsupportedCount
		}
	case registry.QuantityModeFixed:
		if options.Count != nil || options.GenerationMode != "" || options.MaxImages != nil {
			return GenerateOptions{}, ErrParameterNotSupported
		}
	case registry.QuantityModeSequence:
		if options.Count != nil {
			return GenerateOptions{}, ErrParameterNotSupported
		}
		if options.GenerationMode == "" {
			options.GenerationMode = "single"
		}
		switch options.GenerationMode {
		case "single":
			if options.MaxImages != nil {
				return GenerateOptions{}, ErrMaxImagesNotAllowed
			}
		case "sequence":
			if options.MaxImages == nil {
				return GenerateOptions{}, ErrMaxImagesRequired
			}
			if *options.MaxImages < profile.Quantity.Min || *options.MaxImages > profile.Quantity.Max {
				return GenerateOptions{}, ErrMaxImagesOutOfRange
			}
		default:
			return GenerateOptions{}, ErrGenerationModeInvalid
		}
	default:
		return GenerateOptions{}, ErrParameterNotSupported
	}
	return options, nil
}

func profileSupportsSize(profile *registry.SizeProfile, value string) bool {
	for _, option := range profile.Options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func buildAppContext(scope Scope, conversationID uuid.UUID) (*llmclient.AppContext, error) {
	if scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil || conversationID == uuid.Nil {
		return nil, ErrBillingContextRequired
	}
	sessionID := conversationID.String()
	appCtx := &llmclient.AppContext{
		OrganizationID:     scope.OrganizationID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              sessionID,
		AppType:            imageRuntimeAppType,
		AccountID:          scope.AccountID.String(),
		SessionID:          sessionID,
		ConversationID:     sessionID,
	}
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		appCtx.WorkspaceID = scope.WorkspaceID.String()
	}
	return appCtx, nil
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
		ErrParameterNotSupported,
		ErrUnsupportedSize,
		ErrUnsupportedCount,
		ErrGenerationModeInvalid,
		ErrMaxImagesRequired,
		ErrMaxImagesNotAllowed,
		ErrMaxImagesOutOfRange,
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
