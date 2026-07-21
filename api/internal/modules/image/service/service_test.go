package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntimerepository "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/imageasset"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type fakeAvailableModels struct {
	items []*llmmodelsvc.AvailableModel
}

func (f *fakeAvailableModels) ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelsvc.AvailableModel, error) {
	return f.items, nil
}

func (f *fakeAvailableModels) RefreshCache(context.Context, uuid.UUID) error { return nil }
func (f *fakeAvailableModels) InvalidateTenantCache(uuid.UUID)               {}
func (f *fakeAvailableModels) InvalidateGlobalCache()                        {}
func (f *fakeAvailableModels) SetOfficialRouteBootstrapper(interfaces.OfficialRouteBootstrapper) {
}

type fakeRouteLister struct {
	routes map[string][]*channelmodel.RouteQueryResult
}

func (f fakeRouteLister) GetRoutesForModel(_ context.Context, _ uuid.UUID, modelName string) ([]*channelmodel.RouteQueryResult, error) {
	if f.routes != nil {
		return f.routes[modelName], nil
	}
	return []*channelmodel.RouteQueryResult{{RouteID: uuid.New()}}, nil
}

func TestListModelsReturnsRegisteredAvailableImageModels(t *testing.T) {
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{
			{Provider: "openai", Name: "gpt-image-2"},
			{Provider: "qwen", Name: "qwen-image"},
			{Provider: "qwen", Name: "qwen-image-2.0"},
		}},
		fakeRouteLister{},
		nil,
		nil,
		nil,
	)

	models, err := svc.ListModels(context.Background(), Scope{OrganizationID: uuid.New()})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	want := map[string]bool{
		"openai/gpt-image-2":  false,
		"qwen/qwen-image":     false,
		"qwen/qwen-image-2.0": false,
	}
	for _, model := range models {
		key := model.Provider + "/" + model.Model
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("ListModels missing %s in %#v", key, models)
		}
	}
}

func TestListModelsIncludesRegisteredModelWhenOnlyRouteIsAvailable(t *testing.T) {
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{
			{Provider: "openai", Name: "gpt-image-2"},
			{Provider: "qwen", Name: "qwen-image-2.0"},
		}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"qwen-image": {{RouteID: uuid.New()}},
		}},
		nil,
		nil,
		nil,
	)

	models, err := svc.ListModels(context.Background(), Scope{OrganizationID: uuid.New()})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	for _, model := range models {
		if model.Provider == "qwen" && model.Model == "qwen-image" {
			return
		}
	}
	t.Fatalf("ListModels missing qwen/qwen-image in %#v", models)
}

func TestImageResponseFormatOmitsURLForOpenAIGPTImage(t *testing.T) {
	got := imageResponseFormat(registry.ImageModel{Provider: "openai", Model: "gpt-image-2"})
	if got != "" {
		t.Fatalf("imageResponseFormat() = %q, want empty for OpenAI GPT image models", got)
	}
}

func TestImageResponseFormatUsesURLForOtherImageModels(t *testing.T) {
	got := imageResponseFormat(registry.ImageModel{Provider: "qwen", Model: "qwen-image"})
	if got != "url" {
		t.Fatalf("imageResponseFormat() = %q, want url", got)
	}
}

func TestImageFileFromMetaIncludesLifecycleFields(t *testing.T) {
	expiresAt := int64(123456)
	file := imageFileFromMeta(map[string]interface{}{
		"file_id":         "file-1",
		"tool_file_id":    "file-1",
		"url":             "https://example.com/file.png",
		"download_url":    "https://example.com/file.png?download=1",
		"filename":        "file.png",
		"extension":       ".png",
		"mime_type":       "image/png",
		"transfer_method": "tool_file",
		"lifecycle":       "temporary",
		"expires_at":      expiresAt,
	})
	if file.ToolFileID != "file-1" || file.TransferMethod != "tool_file" || file.Lifecycle != "temporary" {
		t.Fatalf("image file lifecycle fields = %#v", file)
	}
	if file.ExpiresAt == nil || *file.ExpiresAt != expiresAt {
		t.Fatalf("expires_at = %#v, want %d", file.ExpiresAt, expiresAt)
	}
}

type fakeImageLLMClient struct {
	llmclient.LLMClient
	createImageCalls    int
	appCreateImageCalls int
	lastAppCtx          *llmclient.AppContext
	response            *adapter.ImageResponse
	err                 error
}

func (f *fakeImageLLMClient) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	f.createImageCalls++
	return nil, errors.New("unexpected CreateImage call")
}

func (f *fakeImageLLMClient) AppCreateImage(_ context.Context, appCtx *llmclient.AppContext, _ *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	f.appCreateImageCalls++
	f.lastAppCtx = appCtx
	if f.err != nil {
		return nil, f.err
	}
	if f.response != nil {
		return f.response, nil
	}
	return &adapter.ImageResponse{Data: []adapter.ImageItem{{URL: "https://example.com/generated.png"}}}, nil
}

type fakeImageChatService struct {
	chatruntime.Service
	conversation            *runtimemodel.Conversation
	createConversationCalls int
	atomicCreateCalls       int
	completedMessageCalls   int
	messageErr              error
}

type countingImageChatService struct {
	chatruntime.Service
	atomicCreateCalls     int
	completedMessageCalls int
}

func (s *countingImageChatService) CreateConversationWithCompletedMessage(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.CreateConversationWithCompletedMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	s.atomicCreateCalls++
	return nil, nil, errors.New("unexpected conversation write")
}

func (s *countingImageChatService) CreateCompletedMessage(context.Context, chatruntime.Scope, chatruntime.CreateCompletedMessageRequest) (*runtimemodel.Message, error) {
	s.completedMessageCalls++
	return nil, errors.New("unexpected message write")
}

func (f *fakeImageChatService) CreateConversationForCaller(_ context.Context, scope chatruntime.Scope, caller chatruntime.Caller, title string) (*runtimemodel.Conversation, error) {
	f.createConversationCalls++
	f.conversation = &runtimemodel.Conversation{
		ID:               uuid.New(),
		OrganizationID:   scope.OrganizationID,
		AccountID:        scope.AccountID,
		WorkspaceID:      scope.WorkspaceID,
		CallerType:       caller.Type,
		ConversationType: caller.ConversationType,
		Title:            title,
	}
	return f.conversation, nil
}

func (f *fakeImageChatService) CreateCompletedMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.CreateCompletedMessageRequest) (*runtimemodel.Message, error) {
	f.completedMessageCalls++
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return &runtimemodel.Message{ID: uuid.New(), ConversationID: req.ConversationID}, nil
}

func (f *fakeImageChatService) GetConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) (*runtimemodel.Conversation, error) {
	return f.conversation, nil
}

func (f *fakeImageChatService) CreateConversationWithCompletedMessage(_ context.Context, scope chatruntime.Scope, caller chatruntime.Caller, req chatruntime.CreateConversationWithCompletedMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	f.atomicCreateCalls++
	if f.messageErr != nil {
		return nil, nil, f.messageErr
	}
	conversationID := req.ConversationID
	if conversationID == uuid.Nil {
		conversationID = uuid.New()
	}
	f.conversation = &runtimemodel.Conversation{
		ID:               conversationID,
		OrganizationID:   scope.OrganizationID,
		AccountID:        scope.AccountID,
		WorkspaceID:      scope.WorkspaceID,
		CallerType:       caller.Type,
		ConversationType: caller.ConversationType,
		Title:            req.Title,
	}
	return f.conversation, &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID}, nil
}

type fakeImageAssetService struct {
	saveCalls   int
	deleteCalls []string
	saveErrAt   int
}

func (f *fakeImageAssetService) SaveGeneratedImage(context.Context, imageasset.SaveRequest) (map[string]interface{}, error) {
	f.saveCalls++
	if f.saveErrAt > 0 && f.saveCalls == f.saveErrAt {
		return nil, errors.New("save failed")
	}
	fileID := "file-1"
	if f.saveCalls > 1 {
		fileID = "file-2"
	}
	return map[string]interface{}{
		"file_id":      fileID,
		"url":          "https://example.com/signed.png",
		"download_url": "https://example.com/signed.png?download=1",
		"filename":     "generated-image.png",
		"extension":    ".png",
		"mime_type":    "image/png",
	}, nil
}

func (f *fakeImageAssetService) DeleteGeneratedImage(_ context.Context, fileID string) error {
	f.deleteCalls = append(f.deleteCalls, fileID)
	return nil
}

func TestGenerateUsesAppCreateImageWithWorkspaceBillingContext(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	llm := &fakeImageLLMClient{}
	chat := &fakeImageChatService{}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		&fakeImageAssetService{},
	)

	_, err := svc.Generate(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Size:     "1024x1024",
		Count:    1,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if llm.createImageCalls != 0 {
		t.Fatalf("CreateImage calls = %d, want 0", llm.createImageCalls)
	}
	if llm.appCreateImageCalls != 1 {
		t.Fatalf("AppCreateImage calls = %d, want 1", llm.appCreateImageCalls)
	}
	if llm.lastAppCtx == nil {
		t.Fatalf("AppCreateImage app context is nil")
	}
	if llm.lastAppCtx.OrganizationID != organizationID.String() {
		t.Fatalf("OrganizationID = %q, want %q", llm.lastAppCtx.OrganizationID, organizationID)
	}
	if llm.lastAppCtx.WorkspaceID != workspaceID.String() {
		t.Fatalf("WorkspaceID = %q, want %q", llm.lastAppCtx.WorkspaceID, workspaceID)
	}
	if llm.lastAppCtx.BillingSubjectType != llmclient.BillingSubjectTypeWorkspace {
		t.Fatalf("BillingSubjectType = %q, want %q", llm.lastAppCtx.BillingSubjectType, llmclient.BillingSubjectTypeWorkspace)
	}
	if llm.lastAppCtx.AccountID != accountID.String() {
		t.Fatalf("AccountID = %q, want %q", llm.lastAppCtx.AccountID, accountID)
	}
	if llm.lastAppCtx.ConversationID != chat.conversation.ID.String() {
		t.Fatalf("ConversationID = %q, want %q", llm.lastAppCtx.ConversationID, chat.conversation.ID)
	}
	if llm.lastAppCtx.SessionID != chat.conversation.ID.String() {
		t.Fatalf("SessionID = %q, want %q", llm.lastAppCtx.SessionID, chat.conversation.ID)
	}
	if llm.lastAppCtx.AppID != chat.conversation.ID.String() {
		t.Fatalf("AppID = %q, want %q", llm.lastAppCtx.AppID, chat.conversation.ID)
	}
	if llm.lastAppCtx.AppType != imageRuntimeAppType {
		t.Fatalf("AppType = %q, want %q", llm.lastAppCtx.AppType, imageRuntimeAppType)
	}
	if chat.createConversationCalls != 0 {
		t.Fatalf("CreateConversationForCaller calls = %d, want 0", chat.createConversationCalls)
	}
	if chat.atomicCreateCalls != 1 {
		t.Fatalf("CreateConversationWithCompletedMessage calls = %d, want 1", chat.atomicCreateCalls)
	}
}

func TestGenerateFailsWithoutWorkspaceBillingContext(t *testing.T) {
	llm := &fakeImageLLMClient{}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		&fakeImageChatService{},
		&fakeImageAssetService{},
	)

	_, err := svc.Generate(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Size:     "1024x1024",
		Count:    1,
	})
	if !errors.Is(err, ErrBillingContextRequired) {
		t.Fatalf("Generate error = %v, want %v", err, ErrBillingContextRequired)
	}
	if llm.createImageCalls != 0 {
		t.Fatalf("CreateImage calls = %d, want 0", llm.createImageCalls)
	}
	if llm.appCreateImageCalls != 0 {
		t.Fatalf("AppCreateImage calls = %d, want 0", llm.appCreateImageCalls)
	}
}

func TestGenerateDoesNotCreateConversationWhenUpstreamFails(t *testing.T) {
	workspaceID := uuid.New()
	llm := &fakeImageLLMClient{err: errors.New("upstream failed")}
	chat := &fakeImageChatService{}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		&fakeImageAssetService{},
	)

	_, err := svc.Generate(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Size:     "1024x1024",
		Count:    1,
	})
	if !errors.Is(err, ErrUpstreamFailed) {
		t.Fatalf("Generate error = %v, want %v", err, ErrUpstreamFailed)
	}
	if chat.createConversationCalls != 0 || chat.atomicCreateCalls != 0 || chat.completedMessageCalls != 0 {
		t.Fatalf("chat writes = create:%d atomic:%d completed:%d, want all 0", chat.createConversationCalls, chat.atomicCreateCalls, chat.completedMessageCalls)
	}
}

func TestGenerateRejectsExistingConversationOutsideCurrentWorkspaceBeforeSideEffects(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	currentWorkspaceID := uuid.New()

	for _, tc := range []struct {
		name                    string
		conversationWorkspaceID *uuid.UUID
	}{
		{name: "another workspace", conversationWorkspaceID: uuidPtr(uuid.New())},
		{name: "legacy conversation without workspace", conversationWorkspaceID: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			llm := &fakeImageLLMClient{}
			assets := &fakeImageAssetService{}
			chat := &fakeImageChatService{conversation: &runtimemodel.Conversation{
				ID:               uuid.New(),
				OrganizationID:   organizationID,
				WorkspaceID:      tc.conversationWorkspaceID,
				AccountID:        accountID,
				CallerType:       runtimemodel.ConversationCallerAIChat,
				ConversationType: runtimemodel.ConversationTypeImage,
			}}
			svc := NewService(
				registry.NewRegistry(),
				&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
				fakeRouteLister{},
				llm,
				chat,
				assets,
			)

			_, err := svc.Generate(context.Background(), Scope{
				OrganizationID: organizationID,
				AccountID:      accountID,
				WorkspaceID:    &currentWorkspaceID,
			}, GenerateRequest{
				Prompt:         "draw a flower",
				Provider:       "qwen",
				Model:          "qwen-image",
				Size:           "1024x1024",
				Count:          1,
				ConversationID: chat.conversation.ID.String(),
			})
			if !errors.Is(err, ErrConversationNotAccessible) {
				t.Fatalf("Generate error = %v, want %v", err, ErrConversationNotAccessible)
			}
			if llm.appCreateImageCalls != 0 || assets.saveCalls != 0 || chat.completedMessageCalls != 0 || chat.atomicCreateCalls != 0 {
				t.Fatalf("side effects = llm:%d save:%d completed:%d atomic:%d, want all 0", llm.appCreateImageCalls, assets.saveCalls, chat.completedMessageCalls, chat.atomicCreateCalls)
			}
		})
	}
}

func TestGenerateRejectsLegacyImageFallbackBeforeSideEffects(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	conversationID := uuid.New()
	now := time.Now()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "members"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT \* FROM "chat_runtime_conversations"`).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`SELECT \* FROM "agents_conversations"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "agent_id", "mode", "name", "inputs", "status", "from_source",
			"from_account_id", "created_by", "dialogue_count", "created_at", "updated_at",
		}).AddRow(
			conversationID,
			pkguuid.GenerateBuiltInWorkflowUUID("imagegen_chat"),
			"chat", "legacy image", `{}`, "normal", "console",
			accountID, accountID, 1, now, now,
		))

	realChatService := chatruntime.NewService(chatruntimerepository.NewRepositories(db), nil)
	chat := &countingImageChatService{Service: realChatService}
	llm := &fakeImageLLMClient{}
	assets := &fakeImageAssetService{}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		assets,
	)

	_, err = svc.Generate(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:         "draw a flower",
		Provider:       "qwen",
		Model:          "qwen-image",
		Size:           "1024x1024",
		Count:          1,
		ConversationID: conversationID.String(),
	})
	if !errors.Is(err, ErrConversationNotAccessible) {
		t.Fatalf("Generate error = %v, want %v", err, ErrConversationNotAccessible)
	}
	if llm.appCreateImageCalls != 0 || assets.saveCalls != 0 || chat.atomicCreateCalls != 0 || chat.completedMessageCalls != 0 {
		t.Fatalf("side effects = llm:%d save:%d atomic:%d completed:%d, want all 0", llm.appCreateImageCalls, assets.saveCalls, chat.atomicCreateCalls, chat.completedMessageCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestGenerateContinuesExistingConversationInCurrentWorkspace(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	llm := &fakeImageLLMClient{}
	assets := &fakeImageAssetService{}
	chat := &fakeImageChatService{conversation: &runtimemodel.Conversation{
		ID:               uuid.New(),
		OrganizationID:   organizationID,
		WorkspaceID:      &workspaceID,
		AccountID:        accountID,
		CallerType:       runtimemodel.ConversationCallerAIChat,
		ConversationType: runtimemodel.ConversationTypeImage,
	}}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		assets,
	)

	_, err := svc.Generate(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:         "draw a flower",
		Provider:       "qwen",
		Model:          "qwen-image",
		Size:           "1024x1024",
		Count:          1,
		ConversationID: chat.conversation.ID.String(),
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if llm.appCreateImageCalls != 1 || assets.saveCalls != 1 || chat.completedMessageCalls != 1 {
		t.Fatalf("side effects = llm:%d save:%d completed:%d, want all 1", llm.appCreateImageCalls, assets.saveCalls, chat.completedMessageCalls)
	}
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	return &value
}

func TestGenerateCleansSavedImagesWhenLaterSaveFails(t *testing.T) {
	workspaceID := uuid.New()
	assets := &fakeImageAssetService{saveErrAt: 2}
	chat := &fakeImageChatService{}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image-2.0"}}},
		fakeRouteLister{},
		&fakeImageLLMClient{response: &adapter.ImageResponse{Data: []adapter.ImageItem{{URL: "https://example.com/1.png"}, {URL: "https://example.com/2.png"}}}},
		chat,
		assets,
	)

	_, err := svc.Generate(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "draw flowers",
		Provider: "qwen",
		Model:    "qwen-image-2.0",
		Size:     "1024x1024",
		Count:    2,
	})
	if !errors.Is(err, ErrImageSaveFailed) {
		t.Fatalf("Generate error = %v, want %v", err, ErrImageSaveFailed)
	}
	if chat.atomicCreateCalls != 0 || chat.completedMessageCalls != 0 {
		t.Fatalf("chat writes = atomic:%d completed:%d, want 0", chat.atomicCreateCalls, chat.completedMessageCalls)
	}
	if len(assets.deleteCalls) != 1 || assets.deleteCalls[0] != "file-1" {
		t.Fatalf("deleted files = %#v, want [file-1]", assets.deleteCalls)
	}
}

func TestGenerateCleansSavedImagesWhenMessageWriteFails(t *testing.T) {
	workspaceID := uuid.New()
	assets := &fakeImageAssetService{}
	chat := &fakeImageChatService{messageErr: errors.New("message write failed")}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		&fakeImageLLMClient{},
		chat,
		assets,
	)

	_, err := svc.Generate(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Size:     "1024x1024",
		Count:    1,
	})
	if err == nil {
		t.Fatalf("Generate error = nil, want message write error")
	}
	if chat.atomicCreateCalls != 1 {
		t.Fatalf("atomic create calls = %d, want 1", chat.atomicCreateCalls)
	}
	if len(assets.deleteCalls) != 1 || assets.deleteCalls[0] != "file-1" {
		t.Fatalf("deleted files = %#v, want [file-1]", assets.deleteCalls)
	}
}
