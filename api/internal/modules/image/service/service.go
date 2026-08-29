package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/imageasset"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxPromptRunes          = 4000
	imageRuntimeAppType     = "image-runtime"
	successMessage          = "已生成图片"
	pendingMessage          = ""
	cancelledMessage        = "图片生成已取消"
	defaultReferencePrompt  = "请基于参考图生成一张新图片。"
	maxImageSearchRunes     = 200
	maxClientRequestIDRunes = 120
	imageCreateTimeout      = 10 * time.Minute
	maxActiveImageTasks     = 2
)

type Service interface {
	ListModels(ctx context.Context, scope Scope) ([]registry.ImageModel, error)
	Generate(ctx context.Context, scope Scope, req GenerateRequest) (*GenerateResult, error)
	CreateTask(ctx context.Context, scope Scope, req GenerateRequest) (*CreateTaskResult, error)
	ListTasks(ctx context.Context, scope Scope, query ListTasksQuery) (*ListTasksResult, error)
	GetTask(ctx context.Context, scope Scope, taskID string) (*ImageTask, error)
	CancelTask(ctx context.Context, scope Scope, taskID string) (*ImageTask, error)
}

type RouteLister interface {
	GetRoutesForModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*channelmodel.RouteQueryResult, error)
}

type ReferenceFileService interface {
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
	GetFileURL(ctx context.Context, fileID string) (string, error)
}

type service struct {
	registry        *registry.Registry
	availableModels llmmodelsvc.AvailableModelsService
	routes          RouteLister
	llmClient       llmclient.LLMClient
	chatService     chatruntime.Service
	imageAssets     imageasset.Service
	fileService     ReferenceFileService
	tasks           *imageTaskRepository
}

type generationConversation struct {
	ID           uuid.UUID
	Title        string
	Existing     *model.Conversation
	ShouldCreate bool
}

type preparedGeneration struct {
	Prompt         string
	Options        GenerateOptions
	ReferenceImage *ReferenceImage
	ModelSpec      registry.ImageModel
	ChatScope      chatruntime.Scope
	Conversation   *generationConversation
	AppCtx         *llmclient.AppContext
	ImageReq       *adapter.ImageRequest
}

func NewService(reg *registry.Registry, availableModels llmmodelsvc.AvailableModelsService, routes RouteLister, llmClient llmclient.LLMClient, chatService chatruntime.Service, imageAssets imageasset.Service, fileServices ...ReferenceFileService) Service {
	var fileService ReferenceFileService
	if len(fileServices) > 0 {
		fileService = fileServices[0]
	}
	return &service{
		registry:        reg,
		availableModels: availableModels,
		routes:          routes,
		llmClient:       llmClient,
		chatService:     chatService,
		imageAssets:     imageAssets,
		fileService:     fileService,
	}
}

func NewServiceWithTasks(db *gorm.DB, reg *registry.Registry, availableModels llmmodelsvc.AvailableModelsService, routes RouteLister, llmClient llmclient.LLMClient, chatService chatruntime.Service, imageAssets imageasset.Service, fileServices ...ReferenceFileService) Service {
	svc := NewService(reg, availableModels, routes, llmClient, chatService, imageAssets, fileServices...)
	if concrete, ok := svc.(*service); ok && db != nil {
		concrete.tasks = newImageTaskRepository(db)
	}
	return svc
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
	prepared, err := s.prepareGeneration(ctx, scope, req)
	if err != nil {
		return nil, err
	}
	return s.runPreparedGeneration(ctx, scope, prepared)
}

func (s *service) prepareGeneration(ctx context.Context, scope Scope, req GenerateRequest) (*preparedGeneration, error) {
	prompt := strings.TrimSpace(req.Prompt)
	referenceImage, err := s.resolveReferenceImage(ctx, scope, req.ReferenceImage)
	if err != nil {
		return nil, err
	}
	if prompt == "" && referenceImage == nil {
		return nil, ErrPromptRequired
	}
	if len([]rune(prompt)) > maxPromptRunes {
		return nil, ErrPromptTooLong
	}
	effectivePrompt := prompt
	if effectivePrompt == "" {
		effectivePrompt = defaultReferencePrompt
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
	conversation, err := s.resolveGenerationConversation(ctx, chatScope, strings.TrimSpace(req.ConversationID), effectivePrompt)
	if err != nil {
		return nil, ErrConversationNotAccessible
	}
	appCtx, err := buildAppContext(scope, conversation.ID)
	if err != nil {
		return nil, err
	}
	imageReq := &adapter.ImageRequest{
		Provider:       modelSpec.Provider,
		Model:          modelSpec.Model,
		Prompt:         effectivePrompt,
		N:              options.Count,
		Size:           options.Size,
		User:           scope.AccountID.String(),
		GenerationMode: options.GenerationMode,
		MaxImages:      options.MaxImages,
	}
	if referenceImage != nil {
		imageReq.ReferenceImageURL = referenceImage.URL
		if shouldDownloadReferenceImageBytes(modelSpec.Provider, routes) {
			content, err := s.fileService.DownloadFile(ctx, referenceImage.FileID)
			if err != nil || len(content) == 0 {
				return nil, ErrReferenceImageInvalid
			}
			imageReq.ReferenceImageBytes = content
			imageReq.ReferenceImageFilename = referenceImage.Filename
			imageReq.ReferenceImageMimeType = referenceImage.MimeType
		}
	}
	return &preparedGeneration{
		Prompt:         effectivePrompt,
		Options:        options,
		ReferenceImage: referenceImage,
		ModelSpec:      modelSpec,
		ChatScope:      chatScope,
		Conversation:   conversation,
		AppCtx:         appCtx,
		ImageReq:       imageReq,
	}, nil
}

func (s *service) runPreparedGeneration(ctx context.Context, scope Scope, prepared *preparedGeneration) (*GenerateResult, error) {
	if prepared == nil || prepared.Conversation == nil || prepared.ImageReq == nil {
		return nil, ErrUpstreamFailed
	}
	generation, err := s.executePreparedGeneration(ctx, scope, prepared)
	if err != nil {
		return nil, err
	}
	files := generation.Files
	messageMetadata := imageGenerationMessageMetadata("", "", generation, "", "")
	messageReq := chatruntime.CreateCompletedMessageRequest{
		ConversationID: prepared.Conversation.ID,
		Query:          prepared.Prompt,
		Answer:         successMessage,
		ModelProvider:  prepared.ModelSpec.Provider,
		ModelName:      prepared.ModelSpec.Model,
		Metadata:       messageMetadata,
	}
	var completed *model.Message
	if prepared.Conversation.ShouldCreate {
		createdConversation, message, createErr := s.chatService.CreateConversationWithCompletedMessage(ctx, prepared.ChatScope, chatruntime.Caller{
			Type:             model.ConversationCallerAIChat,
			ConversationType: model.ConversationTypeImage,
		}, chatruntime.CreateConversationWithCompletedMessageRequest{
			ConversationID: prepared.Conversation.ID,
			Title:          prepared.Conversation.Title,
			Message:        messageReq,
		})
		if createErr != nil {
			if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
				return nil, fmt.Errorf("%w; cleanup failed: %v", createErr, cleanupErr)
			}
			return nil, createErr
		}
		prepared.Conversation.Existing = createdConversation
		completed = message
	} else {
		message, createErr := s.chatService.CreateCompletedMessage(ctx, prepared.ChatScope, messageReq)
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
	if prepared.Conversation.Existing == nil {
		if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
			return nil, fmt.Errorf("image conversation was not created; cleanup failed: %w", cleanupErr)
		}
		return nil, fmt.Errorf("image conversation was not created")
	}
	return &GenerateResult{
		ConversationID:  prepared.Conversation.Existing.ID.String(),
		MessageID:       completed.ID.String(),
		Message:         successMessage,
		ImageGeneration: generation,
	}, nil
}

func (s *service) executePreparedGeneration(ctx context.Context, scope Scope, prepared *preparedGeneration) (ImageGenerationMetadata, error) {
	logCtx := imageRuntimeLogContext(ctx, scope, "", "", prepared)
	resp, err := s.llmClient.AppCreateImage(ctx, prepared.AppCtx, prepared.ImageReq)
	if err != nil {
		return ImageGenerationMetadata{}, fmt.Errorf("%w: %w", ErrUpstreamFailed, err)
	}
	if resp == nil || len(resp.Data) == 0 {
		return ImageGenerationMetadata{}, ErrUpstreamFailed
	}

	files := make([]ImageFile, 0, len(resp.Data))
	conversationID := prepared.Conversation.ID.String()
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
			logger.ErrorContext(logCtx, "image runtime generated image save failed",
				zap.Int("image_index", idx),
				zap.Int("saved_count", len(files)),
				zap.Error(err),
			)
			if cleanupErr := s.cleanupGeneratedFiles(ctx, files); cleanupErr != nil {
				return ImageGenerationMetadata{}, fmt.Errorf("%w: %v; cleanup failed: %v", ErrImageSaveFailed, err, cleanupErr)
			}
			return ImageGenerationMetadata{}, fmt.Errorf("%w: %v", ErrImageSaveFailed, err)
		}
		files = append(files, imageFileFromMeta(fileMeta))
	}

	return ImageGenerationMetadata{
		Provider:       prepared.ModelSpec.Provider,
		Model:          prepared.ModelSpec.Model,
		ModelLabel:     prepared.ModelSpec.ModelLabel,
		Size:           prepared.Options.Size,
		Count:          len(files),
		GenerationMode: prepared.Options.GenerationMode,
		MaxImages:      prepared.Options.MaxImages,
		Files:          files,
		ReferenceImage: prepared.ReferenceImage,
		Status:         "succeeded",
	}, nil
}

func (s *service) CreateTask(ctx context.Context, scope Scope, req GenerateRequest) (*CreateTaskResult, error) {
	if s.tasks == nil {
		result, err := s.Generate(ctx, scope, req)
		if err != nil {
			return nil, err
		}
		return &CreateTaskResult{Task: imageTaskFromGenerateResult(result, req.ClientRequestID)}, nil
	}
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), imageCreateTimeout)
	defer cancel()

	clientRequestID := normalizeImageClientRequestID(req.ClientRequestID)
	if clientRequestID != "" {
		existing, err := s.tasks.findByClientRequestID(operationCtx, scope, clientRequestID)
		if err == nil {
			task := imageTaskFromRecord(*existing)
			return &CreateTaskResult{Task: task}, nil
		}
		if !errors.Is(err, ErrTaskNotFound) {
			return nil, err
		}
	}

	prepared, err := s.prepareGeneration(operationCtx, scope, req)
	if err != nil {
		return nil, err
	}
	taskID := localImageTaskID()
	now := time.Now().UTC()
	record := &imageTaskRecord{
		ID:              uuid.New(),
		OrganizationID:  scope.OrganizationID,
		AccountID:       scope.AccountID,
		WorkspaceID:     scope.WorkspaceID,
		TaskID:          taskID,
		ClientRequestID: clientRequestID,
		ConversationID:  prepared.Conversation.ID.String(),
		Provider:        prepared.ModelSpec.Provider,
		Model:           prepared.ModelSpec.Model,
		ModelLabel:      prepared.ModelSpec.ModelLabel,
		Prompt:          prepared.Prompt,
		Status:          "pending",
		Size:            prepared.Options.Size,
		Count:           normalizedImageTaskCount(prepared.Options),
		GenerationMode:  prepared.Options.GenerationMode,
		MaxImages:       prepared.Options.MaxImages,
		Files:           jsonData([]ImageFile{}),
		ReferenceImage:  jsonData(prepared.ReferenceImage),
		RequestPayload:  jsonData(imageRequestPayload(prepared.ImageReq)),
		ResponsePayload: jsonData(map[string]any{}),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.tasks.createIfActiveBelowLimit(operationCtx, scope, record, maxActiveImageTasks); err != nil {
		if clientRequestID != "" {
			if existing, findErr := s.tasks.findByClientRequestID(operationCtx, scope, clientRequestID); findErr == nil {
				task := imageTaskFromRecord(*existing)
				return &CreateTaskResult{Task: task}, nil
			}
		}
		if errors.Is(err, ErrTaskConflict) {
			return nil, imageTaskConflictError("image.task.create")
		}
		return nil, err
	}
	logger.InfoContext(imageRuntimeLogContext(operationCtx, scope, record.TaskID, record.ClientRequestID, prepared), "image runtime task created",
		zap.String("conversation_id", record.ConversationID),
		zap.String("status", record.Status),
	)
	conversation, message, err := s.createPendingImageMessage(operationCtx, prepared, *record)
	if err != nil {
		failedAt := time.Now().UTC()
		record.Status = "failed"
		record.ErrorMessage = imageErrorMessage(err)
		record.CompletedAt = &failedAt
		record.UpdatedAt = failedAt
		record.ResponsePayload = jsonData(imageTaskErrorPayload(err, record.ErrorMessage))
		_ = s.tasks.save(context.Background(), record)
		logger.ErrorContext(imageRuntimeLogContext(operationCtx, scope, record.TaskID, record.ClientRequestID, prepared), "image runtime pending message creation failed",
			zap.String("public_error", record.ErrorMessage),
			zap.Error(err),
		)
		return nil, err
	}
	prepared.Conversation.ID = conversation.ID
	prepared.Conversation.Existing = conversation
	prepared.Conversation.ShouldCreate = false
	record.ConversationID = conversation.ID.String()
	record.MessageID = message.ID.String()
	if err := s.tasks.save(operationCtx, record); err != nil {
		return nil, err
	}

	s.startImageTask(scope, *record, prepared)
	task := imageTaskFromRecord(*record)
	return &CreateTaskResult{Task: task}, nil
}

func (s *service) createPendingImageMessage(ctx context.Context, prepared *preparedGeneration, record imageTaskRecord) (*model.Conversation, *model.Message, error) {
	if prepared == nil || prepared.Conversation == nil {
		return nil, nil, ErrConversationNotAccessible
	}
	generation := pendingImageGeneration(prepared, "running")
	metadata := imageGenerationMessageMetadata(record.TaskID, record.ClientRequestID, generation, "running", "")
	messageReq := chatruntime.CreatePendingMessageRequest{
		ConversationID: prepared.Conversation.ID,
		Query:          prepared.Prompt,
		Answer:         pendingMessage,
		ModelProvider:  prepared.ModelSpec.Provider,
		ModelName:      prepared.ModelSpec.Model,
		Metadata:       metadata,
	}
	if prepared.Conversation.ShouldCreate {
		return s.chatService.CreateConversationWithPendingMessage(ctx, prepared.ChatScope, chatruntime.Caller{
			Type:             model.ConversationCallerAIChat,
			ConversationType: model.ConversationTypeImage,
		}, chatruntime.CreateConversationWithPendingMessageRequest{
			ConversationID: prepared.Conversation.ID,
			Title:          prepared.Conversation.Title,
			Message:        messageReq,
		})
	}
	message, err := s.chatService.CreatePendingMessage(ctx, prepared.ChatScope, messageReq)
	if err != nil {
		return nil, nil, err
	}
	return prepared.Conversation.Existing, message, nil
}

func (s *service) startImageTask(scope Scope, record imageTaskRecord, prepared *preparedGeneration) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), imageCreateTimeout)
		defer cancel()
		logCtx := imageRuntimeLogContext(ctx, scope, record.TaskID, record.ClientRequestID, prepared)

		started, err := s.tasks.markRunning(ctx, record.TaskID)
		if err != nil || !started {
			logger.WarnContext(logCtx, "image runtime task did not enter running state",
				zap.Bool("started", started),
				zap.Error(err),
			)
			return
		}
		record.Status = "running"
		record.UpdatedAt = time.Now().UTC()
		logger.InfoContext(logCtx, "image runtime task started",
			zap.String("message_id", record.MessageID),
			zap.String("conversation_id", record.ConversationID),
			zap.String("timeout", imageCreateTimeout.String()),
		)

		taskStartedAt := time.Now()
		generation, err := s.executePreparedGeneration(ctx, scope, prepared)
		latest, findErr := s.tasks.findByTaskID(context.Background(), scope, record.TaskID)
		if findErr != nil {
			logger.WarnContext(logCtx, "image runtime task reload failed after upstream call",
				zap.Error(findErr),
			)
		}
		if findErr == nil && latest != nil {
			record = *latest
		}
		if isTerminalImageTaskStatus(record.Status) {
			status := normalizeImageTaskStatus(record.Status)
			logger.InfoContext(logCtx, "image runtime task observed terminal state after upstream call",
				zap.String("status", status),
				zap.Int("generated_file_count", len(generation.Files)),
			)
			if len(generation.Files) > 0 {
				if cleanupErr := s.cleanupGeneratedFiles(context.Background(), generation.Files); cleanupErr != nil {
					logger.WarnContext(logCtx, "image runtime generated files cleanup failed after terminal state",
						zap.Error(cleanupErr),
					)
				}
			}
			return
		}

		now := time.Now().UTC()
		record.UpdatedAt = now
		record.CompletedAt = &now
		if err != nil {
			record.Status = "failed"
			record.ErrorMessage = imageErrorMessage(err)
			record.ResponsePayload = jsonData(imageTaskErrorPayload(err, record.ErrorMessage))
			if saveErr := s.tasks.save(context.Background(), &record); saveErr != nil {
				logger.ErrorContext(logCtx, "image runtime task failure save failed",
					zap.String("public_error", record.ErrorMessage),
					zap.Error(saveErr),
				)
			}
			logger.WarnContext(logCtx, "image runtime task failed",
				zap.String("public_error", record.ErrorMessage),
				zap.Int64("latency_ms", time.Since(taskStartedAt).Milliseconds()),
				zap.Error(err),
			)
			s.markImageMessageFailed(context.Background(), record, prepared, record.ErrorMessage)
			return
		}
		messageID, parseErr := uuid.Parse(strings.TrimSpace(record.MessageID))
		conversationID, convParseErr := uuid.Parse(strings.TrimSpace(record.ConversationID))
		if parseErr != nil || convParseErr != nil {
			record.Status = "failed"
			record.ErrorMessage = ErrConversationNotAccessible.Error()
			record.ResponsePayload = jsonData(imageTaskErrorPayload(ErrConversationNotAccessible, record.ErrorMessage))
			_ = s.cleanupGeneratedFiles(context.Background(), generation.Files)
			_ = s.tasks.save(context.Background(), &record)
			logger.ErrorContext(logCtx, "image runtime task cannot complete message because ids are invalid",
				zap.String("message_id", record.MessageID),
				zap.String("conversation_id", record.ConversationID),
				zap.Error(errors.Join(parseErr, convParseErr)),
			)
			return
		}
		generation.Status = "succeeded"
		metadata := imageGenerationMessageMetadata(record.TaskID, record.ClientRequestID, generation, "succeeded", "")
		message, err := s.chatService.CompleteMessage(context.Background(), prepared.ChatScope, chatruntime.CompleteMessageRequest{
			ConversationID: conversationID,
			MessageID:      messageID,
			Answer:         successMessage,
			Metadata:       metadata,
		})
		if err != nil {
			record.Status = "failed"
			record.ErrorMessage = imageErrorMessage(err)
			record.ResponsePayload = jsonData(imageTaskErrorPayload(err, record.ErrorMessage))
			_ = s.cleanupGeneratedFiles(context.Background(), generation.Files)
			_ = s.tasks.save(context.Background(), &record)
			logger.ErrorContext(logCtx, "image runtime task message completion failed",
				zap.String("public_error", record.ErrorMessage),
				zap.Int("file_count", len(generation.Files)),
				zap.Error(err),
			)
			s.markImageMessageFailed(context.Background(), record, prepared, record.ErrorMessage)
			return
		}

		record.Status = "succeeded"
		if message != nil {
			record.MessageID = message.ID.String()
			record.ConversationID = message.ConversationID.String()
		}
		record.Files = jsonData(generation.Files)
		record.ResponsePayload = jsonData(map[string]any{"image_generation": generation})
		if saveErr := s.tasks.save(context.Background(), &record); saveErr != nil {
			logger.ErrorContext(logCtx, "image runtime task success save failed",
				zap.Int("file_count", len(generation.Files)),
				zap.Error(saveErr),
			)
			return
		}
		logger.InfoContext(logCtx, "image runtime task succeeded",
			zap.String("message_id", record.MessageID),
			zap.String("conversation_id", record.ConversationID),
			zap.Int("file_count", len(generation.Files)),
		)
	}()
}

func imageRuntimeLogContext(ctx context.Context, scope Scope, taskID, clientRequestID string, prepared *preparedGeneration) context.Context {
	fields := []zap.Field{
		zap.String("organization_id", scope.OrganizationID.String()),
		zap.String("account_id", scope.AccountID.String()),
	}
	if scope.WorkspaceID != nil {
		fields = append(fields, zap.String("workspace_id", scope.WorkspaceID.String()))
	}
	if taskID = strings.TrimSpace(taskID); taskID != "" {
		fields = append(fields, zap.String("image_task_id", taskID))
	}
	if clientRequestID = strings.TrimSpace(clientRequestID); clientRequestID != "" {
		fields = append(fields, zap.String("client_request_id", clientRequestID))
	}
	if prepared != nil {
		fields = append(fields,
			zap.String("provider", prepared.ModelSpec.Provider),
			zap.String("model", prepared.ModelSpec.Model),
			zap.String("model_label", prepared.ModelSpec.ModelLabel),
		)
		if prepared.Conversation != nil {
			fields = append(fields, zap.String("conversation_id", prepared.Conversation.ID.String()))
		}
	}
	return logger.WithFields(ctx, fields...)
}

func (s *service) markImageMessageFailed(ctx context.Context, record imageTaskRecord, prepared *preparedGeneration, message string) {
	if prepared == nil {
		return
	}
	conversationID, convErr := uuid.Parse(strings.TrimSpace(record.ConversationID))
	messageID, msgErr := uuid.Parse(strings.TrimSpace(record.MessageID))
	if convErr != nil || msgErr != nil {
		return
	}
	generation := pendingImageGeneration(prepared, "failed")
	metadata := imageGenerationMessageMetadata(record.TaskID, record.ClientRequestID, generation, "failed", message)
	_, _ = s.chatService.FailMessage(ctx, prepared.ChatScope, chatruntime.FailMessageRequest{
		ConversationID: conversationID,
		MessageID:      messageID,
		ErrorMessage:   message,
		Metadata:       metadata,
	})
}

func (s *service) markImageMessageStopped(ctx context.Context, record imageTaskRecord, prepared *preparedGeneration) {
	if prepared == nil {
		return
	}
	conversationID, convErr := uuid.Parse(strings.TrimSpace(record.ConversationID))
	messageID, msgErr := uuid.Parse(strings.TrimSpace(record.MessageID))
	if convErr != nil || msgErr != nil {
		return
	}
	generation := pendingImageGeneration(prepared, "cancelled")
	metadata := imageGenerationMessageMetadata(record.TaskID, record.ClientRequestID, generation, "cancelled", "")
	_, _ = s.chatService.StopRuntimeMessage(ctx, prepared.ChatScope, chatruntime.StopRuntimeMessageRequest{
		ConversationID: conversationID,
		MessageID:      messageID,
		Answer:         cancelledMessage,
		Metadata:       metadata,
	})
}

type imageTaskCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

func (s *service) ListTasks(ctx context.Context, scope Scope, query ListTasksQuery) (*ListTasksResult, error) {
	if s.tasks == nil {
		return &ListTasksResult{Data: []ImageTask{}, Total: 0, HasMore: false}, nil
	}
	search := strings.TrimSpace(query.Search)
	if len([]rune(search)) > maxImageSearchRunes {
		return nil, imageSearchTooLongError("image.task.list")
	}
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	params := imageTaskListParams{Limit: limit, Search: search}
	if cursor := strings.TrimSpace(query.Cursor); cursor != "" {
		decoded, err := decodeImageTaskCursor(cursor)
		if err != nil {
			return nil, imageInvalidCursorError("image.task.list")
		}
		params.BeforeCreatedAt = &decoded.CreatedAt
		params.BeforeID = &decoded.ID
	}
	page, err := s.tasks.list(ctx, scope, params)
	if err != nil {
		return nil, err
	}
	result := make([]ImageTask, 0, len(page.Records))
	for _, record := range page.Records {
		result = append(result, imageTaskFromRecord(record))
	}
	nextCursor := ""
	if page.HasMore && len(page.Records) > 0 {
		last := page.Records[len(page.Records)-1]
		nextCursor = encodeImageTaskCursor(imageTaskCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return &ListTasksResult{
		Data:       result,
		Total:      page.Total,
		HasMore:    page.HasMore,
		NextCursor: nextCursor,
	}, nil
}

func (s *service) GetTask(ctx context.Context, scope Scope, taskID string) (*ImageTask, error) {
	if s.tasks == nil {
		return nil, imageTaskNotFoundError("image.task.get")
	}
	record, err := s.tasks.findByTaskID(ctx, scope, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil, imageTaskNotFoundError("image.task.get")
		}
		return nil, err
	}
	task := imageTaskFromRecord(*record)
	return &task, nil
}

func (s *service) CancelTask(ctx context.Context, scope Scope, taskID string) (*ImageTask, error) {
	if s.tasks == nil {
		return nil, imageTaskNotFoundError("image.task.cancel")
	}
	record, err := s.tasks.cancelByTaskID(ctx, scope, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil, imageTaskNotFoundError("image.task.cancel")
		}
		return nil, err
	}
	if record.MessageID != "" && record.ConversationID != "" {
		conversationID, convErr := uuid.Parse(strings.TrimSpace(record.ConversationID))
		messageID, msgErr := uuid.Parse(strings.TrimSpace(record.MessageID))
		if convErr == nil && msgErr == nil {
			generation := ImageGenerationMetadata{
				Provider:       record.Provider,
				Model:          record.Model,
				ModelLabel:     record.ModelLabel,
				Size:           record.Size,
				Count:          record.Count,
				GenerationMode: record.GenerationMode,
				MaxImages:      record.MaxImages,
				Files:          []ImageFile{},
				ReferenceImage: referenceImageFromJSON(record.ReferenceImage),
				Status:         "cancelled",
			}
			metadata := imageGenerationMessageMetadata(record.TaskID, record.ClientRequestID, generation, "cancelled", "")
			_, _ = s.chatService.StopRuntimeMessage(ctx, chatruntime.Scope{OrganizationID: scope.OrganizationID, AccountID: scope.AccountID, WorkspaceID: scope.WorkspaceID}, chatruntime.StopRuntimeMessageRequest{
				ConversationID: conversationID,
				MessageID:      messageID,
				Answer:         cancelledMessage,
				Metadata:       metadata,
			})
		}
	}
	task := imageTaskFromRecord(*record)
	return &task, nil
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

func shouldDownloadReferenceImageBytes(provider string, routes []*channelmodel.RouteQueryResult) bool {
	if strings.EqualFold(strings.TrimSpace(provider), "openai") {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(provider), "qwen") {
		return false
	}
	for _, route := range routes {
		if route == nil || !route.NativeProtocols.OpenAIResponses.Enabled {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(route.ChannelProvider)) {
		case "qwen", "openai-compatible", "agicto":
			return true
		}
	}
	return false
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

func (s *service) resolveReferenceImage(ctx context.Context, scope Scope, input *ReferenceImage) (*ReferenceImage, error) {
	if input == nil || strings.TrimSpace(input.FileID) == "" {
		return nil, nil
	}
	if s.fileService == nil {
		return nil, ErrReferenceImageUnsupported
	}
	fileID := strings.TrimSpace(input.FileID)
	file, err := s.fileService.GetFileByID(ctx, fileID)
	if err != nil || file == nil {
		return nil, ErrReferenceImageInvalid
	}
	if !fileBelongsToScope(file, scope) {
		return nil, ErrReferenceImageInvalid
	}
	if scope.WorkspaceID != nil && file.WorkspaceID != nil && strings.TrimSpace(*file.WorkspaceID) != "" && strings.TrimSpace(*file.WorkspaceID) != scope.WorkspaceID.String() {
		return nil, ErrReferenceImageInvalid
	}
	if !isSupportedReferenceImage(file.MimeType, file.Extension) {
		return nil, ErrReferenceImageUnsupported
	}
	url, err := s.fileService.GetFileURL(ctx, fileID)
	if err != nil || strings.TrimSpace(url) == "" {
		return nil, ErrReferenceImageInvalid
	}
	return &ReferenceImage{
		FileID:   file.ID,
		URL:      strings.TrimSpace(url),
		Filename: strings.TrimSpace(file.Name),
		MimeType: strings.TrimSpace(file.MimeType),
	}, nil
}

func fileBelongsToScope(file *dto.UploadFile, scope Scope) bool {
	if file == nil {
		return false
	}
	scopeOrganizationID := strings.TrimSpace(scope.OrganizationID.String())
	if strings.TrimSpace(file.OrganizationID) == scopeOrganizationID || strings.TrimSpace(file.TenantID) == scopeOrganizationID {
		return true
	}
	if !file.IsTemporary || !isZeroUUIDString(file.OrganizationID) {
		return false
	}
	return strings.TrimSpace(string(file.CreatedByRole)) == string(dto.CreatedByRoleAccount) &&
		strings.TrimSpace(file.CreatedBy) == strings.TrimSpace(scope.AccountID.String())
}

func isZeroUUIDString(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == "00000000-0000-0000-0000-000000000000"
}

func isSupportedReferenceImage(mimeType, extension string) bool {
	normalizedMime := strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(normalizedMime, "image/") {
		return true
	}
	switch strings.ToLower(strings.Trim(strings.TrimSpace(extension), ".")) {
	case "jpg", "jpeg", "png", "webp", "gif":
		return true
	default:
		return false
	}
}

func normalizeImageClientRequestID(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxClientRequestIDRunes {
		return value
	}
	return string(runes[:maxClientRequestIDRunes])
}

func localImageTaskID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 8 {
		id = id[:8]
	}
	return "image-" + time.Now().UTC().Format("20060102150405") + "-" + id
}

func activeImageTaskStatuses() []string {
	return []string{"pending", "running", "processing", "in_progress"}
}

func normalizeImageTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "created":
		return "pending"
	case "running", "processing", "in_progress":
		return "running"
	case "succeeded", "success", "completed", "done":
		return "succeeded"
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func isTerminalImageTaskStatus(status string) bool {
	switch normalizeImageTaskStatus(status) {
	case "succeeded", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func normalizedImageTaskCount(options GenerateOptions) int {
	if options.GenerationMode == "sequence" && options.MaxImages != nil {
		return *options.MaxImages
	}
	if options.Count != nil {
		return *options.Count
	}
	return 1
}

func pendingImageGeneration(prepared *preparedGeneration, status string) ImageGenerationMetadata {
	if prepared == nil {
		return ImageGenerationMetadata{Status: normalizeImageTaskStatus(status), Files: []ImageFile{}, Count: 1}
	}
	return ImageGenerationMetadata{
		Provider:       prepared.ModelSpec.Provider,
		Model:          prepared.ModelSpec.Model,
		ModelLabel:     prepared.ModelSpec.ModelLabel,
		Size:           prepared.Options.Size,
		Count:          normalizedImageTaskCount(prepared.Options),
		GenerationMode: prepared.Options.GenerationMode,
		MaxImages:      prepared.Options.MaxImages,
		Files:          []ImageFile{},
		ReferenceImage: prepared.ReferenceImage,
		Status:         normalizeImageTaskStatus(status),
	}
}

func imageGenerationMessageMetadata(taskID, clientRequestID string, generation ImageGenerationMetadata, status string, errorMessage string) map[string]interface{} {
	normalizedStatus := normalizeImageTaskStatus(status)
	if normalizedStatus == "" {
		normalizedStatus = normalizeImageTaskStatus(generation.Status)
	}
	if normalizedStatus == "" {
		normalizedStatus = "running"
	}
	generation.Status = normalizedStatus
	if generation.Files == nil {
		generation.Files = []ImageFile{}
	}
	metadata := map[string]interface{}{
		"image_generation":   generation,
		"image_task_id":      strings.TrimSpace(taskID),
		"image_task_status":  normalizedStatus,
		"client_request_id":  strings.TrimSpace(clientRequestID),
		"image_runtime_kind": "generation",
	}
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage != "" {
		metadata["image_task_error"] = errorMessage
	}
	return metadata
}

func encodeImageTaskCursor(cursor imageTaskCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeImageTaskCursor(value string) (imageTaskCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return imageTaskCursor{}, err
	}
	var cursor imageTaskCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return imageTaskCursor{}, err
	}
	if cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil {
		return imageTaskCursor{}, errors.New("cursor is incomplete")
	}
	return cursor, nil
}

func jsonData(value any) datatypes.JSON {
	if value == nil {
		return datatypes.JSON([]byte("null"))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}

func mapFromJSON(raw datatypes.JSON) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		return map[string]any{}
	}
	return data
}

func imageFilesFromJSON(raw datatypes.JSON) []ImageFile {
	if len(raw) == 0 {
		return nil
	}
	var files []ImageFile
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil
	}
	return files
}

func referenceImageFromJSON(raw datatypes.JSON) *ReferenceImage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var ref ReferenceImage
	if err := json.Unmarshal(raw, &ref); err != nil || strings.TrimSpace(ref.FileID) == "" {
		return nil
	}
	return &ref
}

func imageRequestPayload(req *adapter.ImageRequest) map[string]any {
	data := map[string]any{}
	raw, _ := json.Marshal(req)
	_ = json.Unmarshal(raw, &data)
	if _, ok := data["reference_image_bytes"]; ok {
		data["reference_image_bytes"] = "[omitted]"
	}
	return data
}

func imageTaskFromRecord(record imageTaskRecord) ImageTask {
	files := imageFilesFromJSON(record.Files)
	referenceImage := referenceImageFromJSON(record.ReferenceImage)
	status := normalizeImageTaskStatus(record.Status)
	task := ImageTask{
		ID:              record.ID.String(),
		TaskID:          record.TaskID,
		ClientRequestID: record.ClientRequestID,
		ConversationID:  record.ConversationID,
		MessageID:       record.MessageID,
		Provider:        record.Provider,
		Model:           record.Model,
		ModelLabel:      record.ModelLabel,
		Prompt:          record.Prompt,
		Status:          status,
		Size:            record.Size,
		Count:           record.Count,
		GenerationMode:  record.GenerationMode,
		MaxImages:       record.MaxImages,
		Files:           files,
		ReferenceImage:  referenceImage,
		ErrorMessage:    record.ErrorMessage,
		RequestPayload:  mapFromJSON(record.RequestPayload),
		ResponsePayload: mapFromJSON(record.ResponsePayload),
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		CompletedAt:     record.CompletedAt,
	}
	if status == "succeeded" {
		task.ImageGeneration = &ImageGenerationMetadata{
			Provider:       record.Provider,
			Model:          record.Model,
			ModelLabel:     record.ModelLabel,
			Size:           record.Size,
			Count:          len(files),
			GenerationMode: record.GenerationMode,
			MaxImages:      record.MaxImages,
			Files:          files,
			ReferenceImage: referenceImage,
			Status:         "succeeded",
		}
	}
	return task
}

func imageTaskFromGenerateResult(result *GenerateResult, clientRequestID string) ImageTask {
	now := time.Now().UTC()
	taskID := localImageTaskID()
	task := ImageTask{
		ID:              result.ConversationID,
		TaskID:          taskID,
		ClientRequestID: normalizeImageClientRequestID(clientRequestID),
		ConversationID:  result.ConversationID,
		MessageID:       result.MessageID,
		Provider:        result.ImageGeneration.Provider,
		Model:           result.ImageGeneration.Model,
		ModelLabel:      result.ImageGeneration.ModelLabel,
		Prompt:          "",
		Status:          "succeeded",
		Size:            result.ImageGeneration.Size,
		Count:           result.ImageGeneration.Count,
		GenerationMode:  result.ImageGeneration.GenerationMode,
		MaxImages:       result.ImageGeneration.MaxImages,
		Files:           result.ImageGeneration.Files,
		ReferenceImage:  result.ImageGeneration.ReferenceImage,
		ImageGeneration: &result.ImageGeneration,
		CreatedAt:       now,
		UpdatedAt:       now,
		CompletedAt:     &now,
	}
	return task
}

func imageErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, adapter.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return ErrTaskTimeout.Error()
	}
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
		ErrTaskTimeout,
		ErrImageSaveFailed,
		ErrReferenceImageRequired,
		ErrReferenceImageInvalid,
		ErrReferenceImageUnsupported,
	} {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}
	return ErrUpstreamFailed.Error()
}

func imageTaskErrorPayload(err error, publicCode string) map[string]any {
	payload := map[string]any{"error": publicCode}
	if detail := imageTaskErrorDetail(err); len(detail) > 0 {
		payload["error_detail"] = detail
	}
	return payload
}

func imageTaskErrorDetail(err error) map[string]any {
	if err == nil {
		return nil
	}
	detail := map[string]any{}
	switch {
	case errors.Is(err, adapter.ErrTimeout) || errors.Is(err, context.DeadlineExceeded):
		detail["kind"] = "timeout"
		detail["message"] = "provider request timed out"
	case errors.Is(err, adapter.ErrAuthFailed):
		detail["kind"] = "auth_failed"
		detail["message"] = "provider authentication failed"
	case errors.Is(err, adapter.ErrRateLimited):
		detail["kind"] = "rate_limited"
		detail["message"] = "provider rate limit exceeded"
	case errors.Is(err, adapter.ErrQuotaExhausted):
		detail["kind"] = "quota_exhausted"
		detail["message"] = "provider quota is exhausted"
	case errors.Is(err, adapter.ErrBillingUnavailable), errors.Is(err, adapter.ErrInsufficientBalance):
		detail["kind"] = "billing_unavailable"
		detail["message"] = "provider billing is unavailable"
	case errors.Is(err, adapter.ErrInvalidRequest):
		detail["kind"] = "invalid_request"
		detail["message"] = "provider rejected the request"
	case errors.Is(err, adapter.ErrContentPolicyViolation):
		detail["kind"] = "content_policy"
		detail["message"] = "provider rejected the content"
	case errors.Is(err, adapter.ErrProxyError):
		detail["kind"] = "proxy_error"
		detail["message"] = "provider proxy returned an error"
	case errors.Is(err, adapter.ErrCapabilityUnsupported):
		detail["kind"] = "capability_unsupported"
		detail["message"] = "provider route does not support this request"
	case errors.Is(err, adapter.ErrUpstreamError), errors.Is(err, ErrUpstreamFailed):
		detail["kind"] = "upstream_error"
		detail["message"] = "provider request failed"
	}

	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) {
		if code := strings.TrimSpace(adapterErr.Code); code != "" {
			detail["provider_code"] = code
		}
		if adapterErr.StatusCode > 0 {
			detail["status_code"] = adapterErr.StatusCode
		}
		if message := strings.TrimSpace(adapterErr.Message); message != "" {
			detail["provider_message"] = message
		}
	}
	return detail
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
		ErrTaskTimeout,
		ErrImageSaveFailed,
		ErrReferenceImageRequired,
		ErrReferenceImageInvalid,
		ErrReferenceImageUnsupported,
	} {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}
	return "IMAGE_RUNTIME_FAILED"
}

func imageTaskNotFoundError(operation string) error {
	return apperror.Wrap(ErrTaskNotFound, AppCodeTaskNotFound, apperror.WithOperation(operation))
}

func imageTaskConflictError(operation string) error {
	return apperror.Wrap(ErrTaskConflict, AppCodeTaskConflict, apperror.WithOperation(operation))
}

func imageSearchTooLongError(operation string) error {
	return apperror.Wrap(ErrSearchTooLong, AppCodeSearchTooLong, apperror.WithOperation(operation))
}

func imageInvalidCursorError(operation string) error {
	return apperror.Wrap(ErrInvalidCursor, AppCodeInvalidCursor, apperror.WithOperation(operation))
}
