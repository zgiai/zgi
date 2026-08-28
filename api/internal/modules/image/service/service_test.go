package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntimerepository "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/imageasset"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
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
	return []*channelmodel.RouteQueryResult{{RouteID: uuid.New(), ChannelProvider: "qwen", Models: []string{modelName}}}, nil
}

func TestListModelsReturnsEveryAvailableImageModel(t *testing.T) {
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{
			{Provider: "openai", Name: "gpt-image-2"},
			{Provider: "qwen", Name: "qwen-image"},
			{Provider: "qwen", Name: "qwen-image-2.0"},
			{Provider: "custom", Name: "future-image-model"},
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
		"openai/gpt-image-2":        false,
		"qwen/qwen-image":           false,
		"qwen/qwen-image-2.0":       false,
		"custom/future-image-model": false,
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

func TestListModelsDoesNotInventRouteOnlyModels(t *testing.T) {
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{
			{Provider: "openai", Name: "gpt-image-2"},
			{Provider: "qwen", Name: "qwen-image-2.0"},
		}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"qwen-image": {{RouteID: uuid.New(), ChannelProvider: "qwen", Models: []string{"qwen-image"}}},
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
			t.Fatalf("ListModels unexpectedly included route-only model in %#v", models)
		}
	}
}

func TestValidateGenerateOptionsRejectsParametersOutsideSafeIntersection(t *testing.T) {
	_, err := validateGenerateOptions(registry.GenerationProfile{}, GenerateOptions{Size: "1024x1024"})
	if !errors.Is(err, ErrParameterNotSupported) {
		t.Fatalf("validateGenerateOptions() error = %v, want %v", err, ErrParameterNotSupported)
	}
}

func TestValidateGenerateOptionsRejectsFixedModelCount(t *testing.T) {
	profile := registry.GenerationProfile{
		Quantity: &registry.QuantityProfile{Mode: registry.QuantityModeFixed, Default: 1},
	}
	_, err := validateGenerateOptions(profile, GenerateOptions{Count: intPtr(2)})
	if !errors.Is(err, ErrParameterNotSupported) {
		t.Fatalf("validateGenerateOptions() error = %v, want %v", err, ErrParameterNotSupported)
	}
}

func TestValidateGenerateOptionsValidatesSequenceSemantics(t *testing.T) {
	profile := registry.GenerationProfile{
		Quantity: &registry.QuantityProfile{Mode: registry.QuantityModeSequence, Default: 1, Min: 2, Max: 15},
	}
	for _, tc := range []struct {
		name    string
		options GenerateOptions
		wantErr error
	}{
		{name: "sequence requires max", options: GenerateOptions{GenerationMode: "sequence"}, wantErr: ErrMaxImagesRequired},
		{name: "single rejects max", options: GenerateOptions{GenerationMode: "single", MaxImages: intPtr(4)}, wantErr: ErrMaxImagesNotAllowed},
		{name: "sequence enforces upper bound", options: GenerateOptions{GenerationMode: "sequence", MaxImages: intPtr(16)}, wantErr: ErrMaxImagesOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateGenerateOptions(profile, tc.options)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validateGenerateOptions() error = %v, want %v", err, tc.wantErr)
			}
		})
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
	lastImageReq        *adapter.ImageRequest
	response            *adapter.ImageResponse
	err                 error
	waitForCreate       <-chan struct{}
}

func (f *fakeImageLLMClient) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	f.createImageCalls++
	return nil, errors.New("unexpected CreateImage call")
}

func (f *fakeImageLLMClient) AppCreateImage(_ context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	f.appCreateImageCalls++
	f.lastAppCtx = appCtx
	f.lastImageReq = req
	if f.waitForCreate != nil {
		<-f.waitForCreate
	}
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
	getConversationErr      error
	createConversationCalls int
	atomicCreateCalls       int
	pendingAtomicCalls      int
	pendingMessageCalls     int
	completedMessageCalls   int
	completeMessageCalls    int
	failedMessageCalls      int
	stoppedMessageCalls     int
	messageErr              error
}

type countingImageChatService struct {
	chatruntime.Service
	atomicCreateCalls     int
	pendingAtomicCalls    int
	pendingMessageCalls   int
	completedMessageCalls int
	completeMessageCalls  int
	failedMessageCalls    int
	stoppedMessageCalls   int
}

func (s *countingImageChatService) CreateConversationWithCompletedMessage(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.CreateConversationWithCompletedMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	s.atomicCreateCalls++
	return nil, nil, errors.New("unexpected conversation write")
}

func (s *countingImageChatService) CreateCompletedMessage(context.Context, chatruntime.Scope, chatruntime.CreateCompletedMessageRequest) (*runtimemodel.Message, error) {
	s.completedMessageCalls++
	return nil, errors.New("unexpected message write")
}

func (s *countingImageChatService) CreateConversationWithPendingMessage(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.CreateConversationWithPendingMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	s.pendingAtomicCalls++
	return nil, nil, errors.New("unexpected pending conversation write")
}

func (s *countingImageChatService) CreatePendingMessage(context.Context, chatruntime.Scope, chatruntime.CreatePendingMessageRequest) (*runtimemodel.Message, error) {
	s.pendingMessageCalls++
	return nil, errors.New("unexpected pending message write")
}

func (s *countingImageChatService) CompleteMessage(context.Context, chatruntime.Scope, chatruntime.CompleteMessageRequest) (*runtimemodel.Message, error) {
	s.completeMessageCalls++
	return nil, errors.New("unexpected runtime message completion")
}

func (s *countingImageChatService) FailMessage(context.Context, chatruntime.Scope, chatruntime.FailMessageRequest) (*runtimemodel.Message, error) {
	s.failedMessageCalls++
	return nil, errors.New("unexpected runtime message failure")
}

func (s *countingImageChatService) StopRuntimeMessage(context.Context, chatruntime.Scope, chatruntime.StopRuntimeMessageRequest) (*runtimemodel.Message, error) {
	s.stoppedMessageCalls++
	return nil, errors.New("unexpected runtime message stop")
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

func (f *fakeImageChatService) CreatePendingMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.CreatePendingMessageRequest) (*runtimemodel.Message, error) {
	f.pendingMessageCalls++
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: req.ConversationID,
		Query:          req.Query,
		Answer:         req.Answer,
		Status:         runtimemodel.MessageStatusStreaming,
		Metadata:       req.Metadata,
	}, nil
}

func (f *fakeImageChatService) CreateCompletedMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.CreateCompletedMessageRequest) (*runtimemodel.Message, error) {
	f.completedMessageCalls++
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return &runtimemodel.Message{ID: uuid.New(), ConversationID: req.ConversationID}, nil
}

func (f *fakeImageChatService) GetConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) (*runtimemodel.Conversation, error) {
	if f.getConversationErr != nil {
		return nil, f.getConversationErr
	}
	return f.conversation, nil
}

func (f *fakeImageChatService) CreateConversationWithPendingMessage(_ context.Context, scope chatruntime.Scope, caller chatruntime.Caller, req chatruntime.CreateConversationWithPendingMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	f.pendingAtomicCalls++
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
	return f.conversation, &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: conversationID,
		Query:          req.Message.Query,
		Answer:         req.Message.Answer,
		Status:         runtimemodel.MessageStatusStreaming,
		Metadata:       req.Message.Metadata,
	}, nil
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

func (f *fakeImageChatService) CompleteMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.CompleteMessageRequest) (*runtimemodel.Message, error) {
	f.completeMessageCalls++
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return &runtimemodel.Message{
		ID:             req.MessageID,
		ConversationID: req.ConversationID,
		Answer:         req.Answer,
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata:       req.Metadata,
	}, nil
}

func (f *fakeImageChatService) FailMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.FailMessageRequest) (*runtimemodel.Message, error) {
	f.failedMessageCalls++
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return &runtimemodel.Message{
		ID:             req.MessageID,
		ConversationID: req.ConversationID,
		Status:         runtimemodel.MessageStatusError,
		Metadata:       req.Metadata,
	}, nil
}

func (f *fakeImageChatService) StopRuntimeMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.StopRuntimeMessageRequest) (*runtimemodel.Message, error) {
	f.stoppedMessageCalls++
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return &runtimemodel.Message{
		ID:             req.MessageID,
		ConversationID: req.ConversationID,
		Answer:         req.Answer,
		Status:         runtimemodel.MessageStatusStopped,
		Metadata:       req.Metadata,
	}, nil
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

type fakeImageReferenceFileService struct {
	file          *dto.UploadFile
	url           string
	content       []byte
	err           error
	downloadErr   error
	downloadCalls int
}

func (f *fakeImageReferenceFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.file, nil
}

func (f *fakeImageReferenceFileService) GetFileURL(context.Context, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.url, nil
}

func (f *fakeImageReferenceFileService) DownloadFile(context.Context, string) ([]byte, error) {
	f.downloadCalls++
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return f.content, nil
}

func newImageTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`
		CREATE TABLE image_runtime_tasks (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			account_id text NOT NULL,
			workspace_id text NULL,
			task_id text NOT NULL,
			client_request_id text NOT NULL DEFAULT '',
			conversation_id text NOT NULL DEFAULT '',
			message_id text NOT NULL DEFAULT '',
			provider text NOT NULL,
			model text NOT NULL,
			model_label text NOT NULL DEFAULT '',
			prompt text NOT NULL DEFAULT '',
			status text NOT NULL DEFAULT 'pending',
			size text NOT NULL DEFAULT '',
			count integer NOT NULL DEFAULT 1,
			generation_mode text NOT NULL DEFAULT '',
			max_images integer NULL,
			files JSON NOT NULL DEFAULT '[]',
			reference_image JSON NOT NULL DEFAULT 'null',
			error_message text NOT NULL DEFAULT '',
			request_payload JSON NOT NULL DEFAULT '{}',
			response_payload JSON NOT NULL DEFAULT '{}',
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			completed_at datetime NULL
		)
	`).Error; err != nil {
		t.Fatalf("migrate image task table: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX idx_image_runtime_tasks_task_id ON image_runtime_tasks (task_id)`).Error; err != nil {
		t.Fatalf("create image task id index: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX idx_image_runtime_tasks_client_request ON image_runtime_tasks (organization_id, account_id, client_request_id) WHERE client_request_id <> ''`).Error; err != nil {
		t.Fatalf("create image client request index: %v", err)
	}
	return db
}

func newImageTaskPollerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newImageTaskTestDB(t)
	if err := db.AutoMigrate(&runtimemodel.Conversation{}, &runtimemodel.Message{}); err != nil {
		t.Fatalf("migrate chat runtime tables: %v", err)
	}
	return db
}

func TestImageTaskPollerMarksExpiredMessageFailed(t *testing.T) {
	db := newImageTaskPollerTestDB(t)
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}
	conversationID := uuid.New()
	messageID := uuid.New()
	taskID := "image-expired-test"
	now := time.Now().UTC()
	older := now.Add(-11 * time.Minute)

	messageMetadata := map[string]interface{}{
		"image_runtime_kind": "generation",
		"image_task_id":      taskID,
		"image_task_status":  "running",
		"image_generation": map[string]interface{}{
			"status": "running",
			"files":  []interface{}{},
		},
	}
	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       scope.OrganizationID,
		AccountID:            scope.AccountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		ConversationType:     runtimemodel.ConversationTypeImage,
		Title:                "image",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusStreaming,
		CurrentLeafMessageID: &messageID,
		ActiveMessageID:      &messageID,
		Source:               runtimemodel.ConversationSourceConsole,
		Metadata:             map[string]interface{}{},
		CreatedAt:            older,
		UpdatedAt:            older,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "draw",
		Answer:         "",
		Status:         runtimemodel.MessageStatusStreaming,
		ModelName:      "gpt-image-2",
		Metadata:       messageMetadata,
		CreatedAt:      older,
		UpdatedAt:      older,
	}
	record := &imageTaskRecord{
		ID:              uuid.New(),
		OrganizationID:  scope.OrganizationID,
		AccountID:       scope.AccountID,
		TaskID:          taskID,
		ClientRequestID: "image-request-expired-test",
		ConversationID:  conversationID.String(),
		MessageID:       messageID.String(),
		Provider:        "openai",
		Model:           "gpt-image-2",
		ModelLabel:      "GPT Image 2",
		Prompt:          "draw",
		Status:          "running",
		Size:            "1024x1024",
		Count:           1,
		Files:           jsonData([]ImageFile{}),
		ReferenceImage:  jsonData(nil),
		RequestPayload:  jsonData(map[string]any{}),
		ResponsePayload: jsonData(map[string]any{}),
		CreatedAt:       older,
		UpdatedAt:       older,
	}

	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	poller := NewTaskPoller(db)
	poller.poll(context.Background())

	var gotTask imageTaskRecord
	if err := db.Where("task_id = ?", taskID).Take(&gotTask).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if gotTask.Status != "failed" || gotTask.ErrorMessage != ErrTaskTimeout.Error() || gotTask.CompletedAt == nil {
		t.Fatalf("task status/error/completed = %q/%q/%v, want failed/%s/non-nil", gotTask.Status, gotTask.ErrorMessage, gotTask.CompletedAt, ErrTaskTimeout)
	}

	var gotMessage runtimemodel.Message
	if err := db.Where("id = ?", messageID).Take(&gotMessage).Error; err != nil {
		t.Fatalf("load message: %v", err)
	}
	if gotMessage.Status != runtimemodel.MessageStatusError {
		t.Fatalf("message status = %q, want error", gotMessage.Status)
	}
	if gotMessage.Error == nil || *gotMessage.Error != ErrTaskTimeout.Error() {
		t.Fatalf("message error = %v, want %s", gotMessage.Error, ErrTaskTimeout)
	}
	if got := gotMessage.Metadata["image_task_status"]; got != "failed" {
		t.Fatalf("image_task_status = %#v, want failed", got)
	}
	imageGeneration, ok := gotMessage.Metadata["image_generation"].(map[string]interface{})
	if !ok {
		t.Fatalf("image_generation = %#v, want object", gotMessage.Metadata["image_generation"])
	}
	if got := imageGeneration["status"]; got != "failed" {
		t.Fatalf("image_generation.status = %#v, want failed", got)
	}

	var gotConversation runtimemodel.Conversation
	if err := db.Where("id = ?", conversationID).Take(&gotConversation).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if gotConversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusIdle || gotConversation.ActiveMessageID != nil {
		t.Fatalf("conversation runtime/active = %q/%v, want idle/nil", gotConversation.RuntimeStatus, gotConversation.ActiveMessageID)
	}
}

func waitForImageTaskStatus(t *testing.T, svc Service, scope Scope, taskID string, status string) ImageTask {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := svc.GetTask(t.Context(), scope, taskID)
		if err == nil && task != nil && task.Status == status {
			return *task
		}
		time.Sleep(20 * time.Millisecond)
	}
	task, err := svc.GetTask(t.Context(), scope, taskID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	t.Fatalf("task status = %q, want %q", task.Status, status)
	return ImageTask{}
}

func TestCreateTaskReturnsBeforeUpstreamCompletes(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	releaseUpstream := make(chan struct{})
	llm := &fakeImageLLMClient{waitForCreate: releaseUpstream}
	chat := &fakeImageChatService{}
	svc := NewServiceWithTasks(
		newImageTaskTestDB(t),
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		&fakeImageAssetService{},
	)
	scope := Scope{OrganizationID: organizationID, AccountID: accountID}

	resultCh := make(chan *CreateTaskResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.CreateTask(t.Context(), scope, GenerateRequest{
			Prompt:   "draw a flower",
			Provider: "qwen",
			Model:    "qwen-image",
			Options:  GenerateOptions{Size: "1664x928"},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case err := <-errCh:
		t.Fatalf("CreateTask returned error: %v", err)
	case result := <-resultCh:
		if result.Task.Status != "pending" {
			t.Fatalf("created task status = %q, want pending", result.Task.Status)
		}
		close(releaseUpstream)
		task := waitForImageTaskStatus(t, svc, scope, result.Task.TaskID, "succeeded")
		if task.MessageID == "" || task.ConversationID == "" || len(task.Files) != 1 {
			t.Fatalf("completed task missing result data: %#v", task)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("CreateTask did not return before upstream completed")
	}
}

func TestCreateTaskIsIdempotentByClientRequestID(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	llm := &fakeImageLLMClient{}
	svc := NewServiceWithTasks(
		newImageTaskTestDB(t),
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		&fakeImageChatService{},
		&fakeImageAssetService{},
	)
	scope := Scope{OrganizationID: organizationID, AccountID: accountID}
	req := GenerateRequest{
		Prompt:          "draw a flower",
		Provider:        "qwen",
		Model:           "qwen-image",
		ClientRequestID: "same-request",
		Options:         GenerateOptions{Size: "1664x928"},
	}

	first, err := svc.CreateTask(t.Context(), scope, req)
	if err != nil {
		t.Fatalf("first CreateTask returned error: %v", err)
	}
	second, err := svc.CreateTask(t.Context(), scope, req)
	if err != nil {
		t.Fatalf("second CreateTask returned error: %v", err)
	}
	if first.Task.TaskID != second.Task.TaskID {
		t.Fatalf("second task id = %q, want %q", second.Task.TaskID, first.Task.TaskID)
	}
	waitForImageTaskStatus(t, svc, scope, first.Task.TaskID, "succeeded")
	if llm.appCreateImageCalls != 1 {
		t.Fatalf("AppCreateImage calls = %d, want 1", llm.appCreateImageCalls)
	}
}

func TestCreateTaskDoesNotOverwriteCancelledTaskAfterUpstreamCompletes(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	releaseUpstream := make(chan struct{})
	llm := &fakeImageLLMClient{waitForCreate: releaseUpstream}
	chat := &fakeImageChatService{}
	assets := &fakeImageAssetService{}
	svc := NewServiceWithTasks(
		newImageTaskTestDB(t),
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		assets,
	)
	scope := Scope{OrganizationID: organizationID, AccountID: accountID}

	result, err := svc.CreateTask(t.Context(), scope, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Options:  GenerateOptions{Size: "1664x928"},
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	waitForImageTaskStatus(t, svc, scope, result.Task.TaskID, "running")

	cancelled, err := svc.CancelTask(t.Context(), scope, result.Task.TaskID)
	if err != nil {
		t.Fatalf("CancelTask returned error: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancelled task status = %q, want cancelled", cancelled.Status)
	}
	close(releaseUpstream)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := svc.GetTask(t.Context(), scope, result.Task.TaskID)
		if err != nil {
			t.Fatalf("GetTask returned error: %v", err)
		}
		if task.Status != "cancelled" {
			t.Fatalf("task status = %q, want cancelled", task.Status)
		}
		if len(assets.deleteCalls) == 1 {
			if assets.deleteCalls[0] != "file-1" {
				t.Fatalf("deleted files = %#v, want [file-1]", assets.deleteCalls)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(assets.deleteCalls) != 1 {
		t.Fatalf("deleted files = %#v, want [file-1]", assets.deleteCalls)
	}
	if chat.completeMessageCalls != 0 {
		t.Fatalf("CompleteMessage calls = %d, want 0", chat.completeMessageCalls)
	}
	if chat.stoppedMessageCalls != 1 {
		t.Fatalf("StopRuntimeMessage calls = %d, want 1", chat.stoppedMessageCalls)
	}
}

func TestGenerateUsesAppCreateImageWithOrganizationBillingContext(t *testing.T) {
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

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Options:  GenerateOptions{Size: "1664x928"},
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
	if llm.lastImageReq == nil {
		t.Fatalf("AppCreateImage image request is nil")
	}
	if llm.lastImageReq.Provider != "qwen" {
		t.Fatalf("image request provider = %q, want %q", llm.lastImageReq.Provider, "qwen")
	}
	if llm.lastAppCtx.OrganizationID != organizationID.String() {
		t.Fatalf("OrganizationID = %q, want %q", llm.lastAppCtx.OrganizationID, organizationID)
	}
	if llm.lastAppCtx.WorkspaceID != workspaceID.String() {
		t.Fatalf("WorkspaceID = %q, want %q", llm.lastAppCtx.WorkspaceID, workspaceID)
	}
	if llm.lastAppCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("BillingSubjectType = %q, want %q", llm.lastAppCtx.BillingSubjectType, llmclient.BillingSubjectTypeOrganization)
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

func TestGeneratePassesReferenceImageURLToLLM(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	fileID := uuid.New().String()
	workspaceIDText := workspaceID.String()
	llm := &fakeImageLLMClient{}
	chat := &fakeImageChatService{}
	files := &fakeImageReferenceFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID.String(),
			TenantID:       organizationID.String(),
			WorkspaceID:    &workspaceIDText,
			Name:           "reference.png",
			Extension:      "png",
			MimeType:       "image/png",
		},
		url: "https://files.example.com/reference.png?sign=1",
	}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "doubao", Name: "doubao-seedream-4-0-250828"}}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"doubao-seedream-4-0-250828": {
				{RouteID: uuid.New(), ChannelProvider: "doubao", Models: []string{"doubao-seedream-4-0-250828"}},
			},
		}},
		llm,
		chat,
		&fakeImageAssetService{},
		files,
	)

	result, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "换成赛博朋克风",
		Provider: "doubao",
		Model:    "doubao-seedream-4-0-250828",
		Options:  GenerateOptions{Size: "1024x1024"},
		ReferenceImage: &ReferenceImage{
			FileID: fileID,
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if llm.lastImageReq == nil {
		t.Fatal("image request is nil")
	}
	if llm.lastImageReq.ReferenceImageURL != files.url {
		t.Fatalf("ReferenceImageURL = %q, want %q", llm.lastImageReq.ReferenceImageURL, files.url)
	}
	if files.downloadCalls != 0 {
		t.Fatalf("DownloadFile calls = %d, want 0 for doubao reference images", files.downloadCalls)
	}
	if result.ImageGeneration.ReferenceImage == nil {
		t.Fatal("result reference image is nil")
	}
	if result.ImageGeneration.ReferenceImage.FileID != fileID {
		t.Fatalf("reference file id = %q, want %q", result.ImageGeneration.ReferenceImage.FileID, fileID)
	}
}

func TestGenerateDownloadsReferenceImageBytesForOpenAI(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	fileID := uuid.New().String()
	workspaceIDText := workspaceID.String()
	llm := &fakeImageLLMClient{}
	chat := &fakeImageChatService{}
	content := []byte("PNGDATA")
	files := &fakeImageReferenceFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID.String(),
			TenantID:       organizationID.String(),
			WorkspaceID:    &workspaceIDText,
			Name:           "reference.png",
			Extension:      "png",
			MimeType:       "image/png",
		},
		url:     "https://files.example.com/reference.png?sign=1",
		content: content,
	}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "openai", Name: "gpt-image-2"}}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"gpt-image-2": {
				{RouteID: uuid.New(), ChannelProvider: "openai", Models: []string{"gpt-image-2"}},
			},
		}},
		llm,
		chat,
		&fakeImageAssetService{},
		files,
	)

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "add a person",
		Provider: "openai",
		Model:    "gpt-image-2",
		Options:  GenerateOptions{Size: "auto"},
		ReferenceImage: &ReferenceImage{
			FileID: fileID,
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if files.downloadCalls != 1 {
		t.Fatalf("DownloadFile calls = %d, want 1", files.downloadCalls)
	}
	if llm.lastImageReq == nil {
		t.Fatal("image request is nil")
	}
	if string(llm.lastImageReq.ReferenceImageBytes) != string(content) {
		t.Fatalf("ReferenceImageBytes = %q, want %q", llm.lastImageReq.ReferenceImageBytes, content)
	}
	if llm.lastImageReq.ReferenceImageFilename != "reference.png" {
		t.Fatalf("ReferenceImageFilename = %q, want reference.png", llm.lastImageReq.ReferenceImageFilename)
	}
	if llm.lastImageReq.ReferenceImageMimeType != "image/png" {
		t.Fatalf("ReferenceImageMimeType = %q, want image/png", llm.lastImageReq.ReferenceImageMimeType)
	}
	if llm.lastImageReq.ReferenceImageURL != files.url {
		t.Fatalf("ReferenceImageURL = %q, want %q", llm.lastImageReq.ReferenceImageURL, files.url)
	}
}

func TestGenerateReturnsInvalidReferenceImageWhenOpenAIDownloadFails(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	fileID := uuid.New().String()
	workspaceIDText := workspaceID.String()
	llm := &fakeImageLLMClient{}
	chat := &fakeImageChatService{}
	files := &fakeImageReferenceFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID.String(),
			TenantID:       organizationID.String(),
			WorkspaceID:    &workspaceIDText,
			Name:           "reference.png",
			Extension:      "png",
			MimeType:       "image/png",
		},
		url:         "https://files.example.com/reference.png?sign=1",
		downloadErr: errors.New("download failed"),
	}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "openai", Name: "gpt-image-2"}}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"gpt-image-2": {
				{RouteID: uuid.New(), ChannelProvider: "openai", Models: []string{"gpt-image-2"}},
			},
		}},
		llm,
		chat,
		&fakeImageAssetService{},
		files,
	)

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "add a person",
		Provider: "openai",
		Model:    "gpt-image-2",
		Options:  GenerateOptions{Size: "auto"},
		ReferenceImage: &ReferenceImage{
			FileID: fileID,
		},
	})
	if !errors.Is(err, ErrReferenceImageInvalid) {
		t.Fatalf("Generate error = %v, want ErrReferenceImageInvalid", err)
	}
	if files.downloadCalls != 1 {
		t.Fatalf("DownloadFile calls = %d, want 1", files.downloadCalls)
	}
	if llm.appCreateImageCalls != 0 {
		t.Fatalf("AppCreateImage calls = %d, want 0", llm.appCreateImageCalls)
	}
}

func TestGenerateKeepsReferenceImageURLOnlyForQwen(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	fileID := uuid.New().String()
	workspaceIDText := workspaceID.String()
	llm := &fakeImageLLMClient{}
	chat := &fakeImageChatService{}
	files := &fakeImageReferenceFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID.String(),
			TenantID:       organizationID.String(),
			WorkspaceID:    &workspaceIDText,
			Name:           "reference.png",
			Extension:      "png",
			MimeType:       "image/png",
		},
		url: "https://files.example.com/reference.png?sign=1",
	}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image-2.0"}}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"qwen-image-2.0": {
				{RouteID: uuid.New(), ChannelProvider: "qwen", Models: []string{"qwen-image-2.0"}},
			},
		}},
		llm,
		chat,
		&fakeImageAssetService{},
		files,
	)

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "换成赛博朋克风",
		Provider: "qwen",
		Model:    "qwen-image-2.0",
		Options:  GenerateOptions{Size: "2048x2048"},
		ReferenceImage: &ReferenceImage{
			FileID: fileID,
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if files.downloadCalls != 0 {
		t.Fatalf("DownloadFile calls = %d, want 0 for qwen reference images", files.downloadCalls)
	}
	if llm.lastImageReq == nil {
		t.Fatal("image request is nil")
	}
	if llm.lastImageReq.ReferenceImageURL != files.url {
		t.Fatalf("ReferenceImageURL = %q, want %q", llm.lastImageReq.ReferenceImageURL, files.url)
	}
	if len(llm.lastImageReq.ReferenceImageBytes) != 0 {
		t.Fatalf("ReferenceImageBytes length = %d, want 0", len(llm.lastImageReq.ReferenceImageBytes))
	}
}

func TestGenerateDownloadsReferenceImageBytesForQwenOpenAIProtocolRoute(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	fileID := uuid.New().String()
	workspaceIDText := workspaceID.String()
	llm := &fakeImageLLMClient{}
	chat := &fakeImageChatService{}
	content := []byte("PNGDATA")
	files := &fakeImageReferenceFileService{
		file: &dto.UploadFile{
			ID:             fileID,
			OrganizationID: organizationID.String(),
			TenantID:       organizationID.String(),
			WorkspaceID:    &workspaceIDText,
			Name:           "reference.png",
			Extension:      "png",
			MimeType:       "image/png",
		},
		url:     "https://files.example.com/reference.png?sign=1",
		content: content,
	}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image-2.0"}}},
		fakeRouteLister{routes: map[string][]*channelmodel.RouteQueryResult{
			"qwen-image-2.0": {
				{
					RouteID:         uuid.New(),
					ChannelProvider: "qwen",
					Models:          []string{"qwen-image-2.0"},
					NativeProtocols: channelmodel.NativeProtocolConfig{
						OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
					},
				},
			},
		}},
		llm,
		chat,
		&fakeImageAssetService{},
		files,
	)

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, GenerateRequest{
		Prompt:   "换成赛博朋克风",
		Provider: "qwen",
		Model:    "qwen-image-2.0",
		Options:  GenerateOptions{Size: "2048x2048"},
		ReferenceImage: &ReferenceImage{
			FileID: fileID,
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if files.downloadCalls != 1 {
		t.Fatalf("DownloadFile calls = %d, want 1", files.downloadCalls)
	}
	if llm.lastImageReq == nil {
		t.Fatal("image request is nil")
	}
	if llm.lastImageReq.ReferenceImageURL != files.url {
		t.Fatalf("ReferenceImageURL = %q, want %q", llm.lastImageReq.ReferenceImageURL, files.url)
	}
	if string(llm.lastImageReq.ReferenceImageBytes) != string(content) {
		t.Fatalf("ReferenceImageBytes = %q, want %q", llm.lastImageReq.ReferenceImageBytes, content)
	}
	if llm.lastImageReq.ReferenceImageFilename != "reference.png" {
		t.Fatalf("ReferenceImageFilename = %q, want reference.png", llm.lastImageReq.ReferenceImageFilename)
	}
	if llm.lastImageReq.ReferenceImageMimeType != "image/png" {
		t.Fatalf("ReferenceImageMimeType = %q, want image/png", llm.lastImageReq.ReferenceImageMimeType)
	}
}

func TestFileBelongsToScopeAllowsOwnedTemporaryZeroOrganizationFile(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	file := &dto.UploadFile{
		OrganizationID: "00000000-0000-0000-0000-000000000000",
		IsTemporary:    true,
		CreatedByRole:  dto.CreatedByRoleAccount,
		CreatedBy:      accountID.String(),
	}

	if !fileBelongsToScope(file, Scope{OrganizationID: organizationID, AccountID: accountID}) {
		t.Fatal("temporary zero-organization file owned by account should be accepted")
	}
	if fileBelongsToScope(file, Scope{OrganizationID: organizationID, AccountID: uuid.New()}) {
		t.Fatal("temporary zero-organization file owned by another account should be rejected")
	}
}

func TestGenerateSucceedsWithoutWorkspaceBillingContext(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
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

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, GenerateRequest{
		Prompt:   "draw a flower",
		Provider: "qwen",
		Model:    "qwen-image",
		Options:  GenerateOptions{Size: "1664x928"},
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
		t.Fatal("AppCreateImage app context is nil")
	}
	if llm.lastAppCtx.OrganizationID != organizationID.String() {
		t.Fatalf("OrganizationID = %q, want %q", llm.lastAppCtx.OrganizationID, organizationID)
	}
	if llm.lastAppCtx.WorkspaceID != "" {
		t.Fatalf("WorkspaceID = %q, want empty", llm.lastAppCtx.WorkspaceID)
	}
	if llm.lastAppCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("BillingSubjectType = %q, want %q", llm.lastAppCtx.BillingSubjectType, llmclient.BillingSubjectTypeOrganization)
	}
	if llm.lastAppCtx.AccountID != accountID.String() {
		t.Fatalf("AccountID = %q, want %q", llm.lastAppCtx.AccountID, accountID)
	}
	if chat.atomicCreateCalls != 1 {
		t.Fatalf("CreateConversationWithCompletedMessage calls = %d, want 1", chat.atomicCreateCalls)
	}
}

func TestBuildAppContextRequiresOrganizationAccountAndConversation(t *testing.T) {
	validScope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}
	validConversationID := uuid.New()

	tests := []struct {
		name           string
		scope          Scope
		conversationID uuid.UUID
	}{
		{name: "missing organization", scope: Scope{AccountID: validScope.AccountID}, conversationID: validConversationID},
		{name: "missing account", scope: Scope{OrganizationID: validScope.OrganizationID}, conversationID: validConversationID},
		{name: "missing conversation", scope: validScope, conversationID: uuid.Nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildAppContext(tt.scope, tt.conversationID)
			if !errors.Is(err, ErrBillingContextRequired) {
				t.Fatalf("buildAppContext() error = %v, want %v", err, ErrBillingContextRequired)
			}
		})
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
		Options:  GenerateOptions{Size: "1664x928"},
	})
	if !errors.Is(err, ErrUpstreamFailed) {
		t.Fatalf("Generate error = %v, want %v", err, ErrUpstreamFailed)
	}
	if chat.createConversationCalls != 0 || chat.atomicCreateCalls != 0 || chat.completedMessageCalls != 0 {
		t.Fatalf("chat writes = create:%d atomic:%d completed:%d, want all 0", chat.createConversationCalls, chat.atomicCreateCalls, chat.completedMessageCalls)
	}
}

func TestImageErrorMessageClassifiesProviderTimeout(t *testing.T) {
	err := fmt.Errorf("%w: %w", ErrUpstreamFailed, adapter.ErrTimeout)
	if got := imageErrorMessage(err); got != ErrTaskTimeout.Error() {
		t.Fatalf("imageErrorMessage() = %q, want %q", got, ErrTaskTimeout.Error())
	}
}

func TestImageTaskErrorPayloadIncludesSafeAdapterDetail(t *testing.T) {
	err := fmt.Errorf("%w: %w", ErrUpstreamFailed, adapter.NewAdapterError("InvalidParameter", "provider rejected the request", 400, adapter.ErrInvalidRequest))
	payload := imageTaskErrorPayload(err, imageErrorMessage(err))
	if payload["error"] != ErrUpstreamFailed.Error() {
		t.Fatalf("payload error = %#v, want %q", payload["error"], ErrUpstreamFailed.Error())
	}
	detail, ok := payload["error_detail"].(map[string]any)
	if !ok {
		t.Fatalf("payload error_detail = %#v, want map", payload["error_detail"])
	}
	if detail["kind"] != "invalid_request" || detail["provider_code"] != "InvalidParameter" || detail["status_code"] != 400 {
		t.Fatalf("payload detail = %#v", detail)
	}
}

func TestGenerateAllowsExistingConversationAcrossWorkspaceContexts(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	currentWorkspaceID := uuid.New()

	for _, tc := range []struct {
		name                    string
		conversationWorkspaceID *uuid.UUID
	}{
		{name: "another workspace", conversationWorkspaceID: uuidPtr(uuid.New())},
		{name: "conversation without workspace", conversationWorkspaceID: nil},
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

			_, err := svc.Generate(t.Context(), Scope{
				OrganizationID: organizationID,
				AccountID:      accountID,
				WorkspaceID:    &currentWorkspaceID,
			}, GenerateRequest{
				Prompt:         "draw a flower",
				Provider:       "qwen",
				Model:          "qwen-image",
				Options:        GenerateOptions{Size: "1664x928"},
				ConversationID: chat.conversation.ID.String(),
			})
			if err != nil {
				t.Fatalf("Generate returned error: %v", err)
			}
			if llm.appCreateImageCalls != 1 || assets.saveCalls != 1 || chat.completedMessageCalls != 1 || chat.atomicCreateCalls != 0 {
				t.Fatalf("side effects = llm:%d save:%d completed:%d atomic:%d, want 1,1,1,0", llm.appCreateImageCalls, assets.saveCalls, chat.completedMessageCalls, chat.atomicCreateCalls)
			}
			if llm.lastAppCtx == nil || llm.lastAppCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
				t.Fatalf("billing app context = %#v, want organization subject", llm.lastAppCtx)
			}
		})
	}
}

func TestGenerateRejectsConversationOutsideCallerScopeBeforeSideEffects(t *testing.T) {
	llm := &fakeImageLLMClient{}
	assets := &fakeImageAssetService{}
	chat := &fakeImageChatService{getConversationErr: errors.New("conversation outside caller scope")}
	svc := NewService(
		registry.NewRegistry(),
		&fakeAvailableModels{items: []*llmmodelsvc.AvailableModel{{Provider: "qwen", Name: "qwen-image"}}},
		fakeRouteLister{},
		llm,
		chat,
		assets,
	)

	_, err := svc.Generate(t.Context(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, GenerateRequest{
		Prompt:         "draw a flower",
		Provider:       "qwen",
		Model:          "qwen-image",
		Options:        GenerateOptions{Size: "1664x928"},
		ConversationID: uuid.NewString(),
	})
	if !errors.Is(err, ErrConversationNotAccessible) {
		t.Fatalf("Generate error = %v, want %v", err, ErrConversationNotAccessible)
	}
	if llm.appCreateImageCalls != 0 || assets.saveCalls != 0 || chat.completedMessageCalls != 0 || chat.atomicCreateCalls != 0 {
		t.Fatalf("side effects = llm:%d save:%d completed:%d atomic:%d, want all 0", llm.appCreateImageCalls, assets.saveCalls, chat.completedMessageCalls, chat.atomicCreateCalls)
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
		Options:        GenerateOptions{Size: "1664x928"},
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
		Options:        GenerateOptions{Size: "1664x928"},
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

func intPtr(value int) *int {
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
		Options:  GenerateOptions{Size: "2048x2048", Count: intPtr(2)},
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
		Options:  GenerateOptions{Size: "1664x928"},
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
