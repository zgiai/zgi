package aichat_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	sharedto "github.com/zgiai/ginext/internal/dto"
	aichatdto "github.com/zgiai/ginext/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	"github.com/zgiai/ginext/internal/modules/aichat/repository"
	aichatservice "github.com/zgiai/ginext/internal/modules/aichat/service"
	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/shared/titlegen"
	workspacemodel "github.com/zgiai/ginext/internal/modules/workspace/model"
	redisutil "github.com/zgiai/ginext/pkg/redis"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestService_ChatCreatesConversationAndContinuesFromCurrentLeaf(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	seedMember(t, db, orgID, accountID)
	seedAccountContext(t, db, accountID, &workspaceID)

	fakeLLM := &fakeLLMClient{chunks: []string{"hello", " world"}}
	svc := aichatservice.NewService(repository.NewRepositories(db), fakeLLM)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}

	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query: "Hello?",
		Model: "gpt-test",
		Parameters: map[string]interface{}{
			"temperature": 0.2,
		},
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if prepared.Conversation.WorkspaceID == nil || *prepared.Conversation.WorkspaceID != workspaceID {
		t.Fatalf("workspace_id = %v, want %s", prepared.Conversation.WorkspaceID, workspaceID)
	}
	if prepared.Message.BillingReasonSource == nil || *prepared.Message.BillingReasonSource != aichatmodel.MessageBillingReasonSourceAIChat {
		t.Fatalf("billing_reason_source = %v, want aichat", prepared.Message.BillingReasonSource)
	}

	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "hello world" {
		t.Fatalf("answer = %q, want hello world", result.Answer)
	}
	if len(fakeLLM.appContexts) != 1 {
		t.Fatalf("app context count = %d, want 1", len(fakeLLM.appContexts))
	}
	if fakeLLM.appContexts[0].AppType != aichatmodel.MessageBillingReasonSourceAIChat {
		t.Fatalf("app type = %q, want aichat", fakeLLM.appContexts[0].AppType)
	}
	if fakeLLM.appContexts[0].BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("billing subject type = %q, want organization", fakeLLM.appContexts[0].BillingSubjectType)
	}

	firstID := prepared.Message.ID
	second, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: prepared.Conversation.ID.String(),
		Query:          "Again",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() second error = %v", err)
	}
	if second.Message.ParentID == nil || *second.Message.ParentID != firstID {
		t.Fatalf("parent_id = %v, want %s", second.Message.ParentID, firstID)
	}
	if len(fakeLLM.requests) == 0 {
		t.Fatalf("expected captured llm request")
	}
	lastReq := second.LLMRequest
	if len(lastReq.Messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(lastReq.Messages))
	}
	if lastReq.Messages[1].Role != "user" || lastReq.Messages[1].Content != "Hello?" {
		t.Fatalf("first history user message = %#v", lastReq.Messages[1])
	}
	if lastReq.Messages[2].Role != "assistant" || lastReq.Messages[2].Content != "hello world" {
		t.Fatalf("first history assistant message = %#v", lastReq.Messages[2])
	}
}

func TestService_ChatSetsAndClearsConversationRuntime(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var running aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&running).Error; err != nil {
		t.Fatalf("load running conversation: %v", err)
	}
	if running.RuntimeStatus != aichatmodel.ConversationRuntimeStatusStreaming {
		t.Fatalf("runtime_status = %q, want streaming", running.RuntimeStatus)
	}
	if running.ActiveMessageID == nil || *running.ActiveMessageID != prepared.Message.ID {
		t.Fatalf("active_message_id = %v, want %s", running.ActiveMessageID, prepared.Message.ID)
	}

	_, err = svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: prepared.Conversation.ID.String(),
		Query:          "again",
		Model:          "gpt-test",
	})
	if !errors.Is(err, aichatservice.ErrConversationRunning) {
		t.Fatalf("PrepareChat() concurrent error = %v, want ErrConversationRunning", err)
	}

	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	var completed aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&completed).Error; err != nil {
		t.Fatalf("load completed conversation: %v", err)
	}
	if completed.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", completed.RuntimeStatus)
	}
	if completed.ActiveMessageID != nil {
		t.Fatalf("active_message_id = %v, want nil", completed.ActiveMessageID)
	}
	if completed.CurrentLeafMessageID == nil || *completed.CurrentLeafMessageID != prepared.Message.ID {
		t.Fatalf("current_leaf_message_id = %v, want %s", completed.CurrentLeafMessageID, prepared.Message.ID)
	}
}

func TestService_NewChatUsesGeneratedConversationTitle(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	titleGenerator := &fakeTitleGenerator{title: "周末旅行计划"}
	svc := aichatservice.NewServiceWithTitleGenerator(
		repository.NewRepositories(db),
		&fakeLLMClient{chunks: []string{"done"}},
		titleGenerator,
	)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query: "帮我规划周末去杭州的旅行",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	waitForConversationTitle(t, db, prepared.Conversation.ID, "周末旅行计划")
	calls, lastRequest := titleGenerator.snapshot()
	if calls != 1 {
		t.Fatalf("title generator calls = %d, want 1", calls)
	}
	if lastRequest.AppID != prepared.Conversation.ID.String() || lastRequest.AppType != aichatmodel.MessageBillingReasonSourceAIChat {
		t.Fatalf("title request = %#v, want aichat conversation identity", lastRequest)
	}
}

func TestService_ExistingChatDoesNotGenerateConversationTitle(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversation := &aichatmodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Existing",
		Status:         aichatmodel.ConversationStatusNormal,
		RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	titleGenerator := &fakeTitleGenerator{title: "Should Not Run"}
	svc := aichatservice.NewServiceWithTitleGenerator(
		repository.NewRepositories(db),
		&fakeLLMClient{chunks: []string{"done"}},
		titleGenerator,
	)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: conversation.ID.String(),
		Query:          "continue",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if prepared.Conversation.Title != "Existing" {
		t.Fatalf("conversation title = %q, want existing title", prepared.Conversation.Title)
	}
	calls, _ := titleGenerator.snapshot()
	if calls != 0 {
		t.Fatalf("title generator calls = %d, want 0", calls)
	}
}

func waitForConversationTitle(t *testing.T, db *gorm.DB, conversationID uuid.UUID, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		var stored aichatmodel.Conversation
		if err := db.Where("id = ?", conversationID).Take(&stored).Error; err != nil {
			t.Fatalf("load stored conversation: %v", err)
		}
		if stored.Title == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored title = %q, want %q", stored.Title, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestService_RunPreparedStreamKeepsRunningWhenRequestContextCancels(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fakeLLM := &fakeLLMClient{blockUntilCancel: true, started: make(chan struct{})}
	svc := aichatservice.NewService(repository.NewRepositories(db), fakeLLM)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := svc.RunPreparedStream(requestCtx, prepared, nil)
		done <- err
	}()

	select {
	case <-fakeLLM.started:
	case <-time.After(time.Second):
		t.Fatalf("llm stream did not start")
	}
	cancelRequest()

	select {
	case err := <-done:
		t.Fatalf("RunPreparedStream() returned after request cancel: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	var running aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&running).Error; err != nil {
		t.Fatalf("load running conversation: %v", err)
	}
	if running.RuntimeStatus != aichatmodel.ConversationRuntimeStatusStreaming {
		t.Fatalf("runtime_status = %q, want streaming", running.RuntimeStatus)
	}
	if running.ActiveMessageID == nil || *running.ActiveMessageID != prepared.Message.ID {
		t.Fatalf("active_message_id = %v, want %s", running.ActiveMessageID, prepared.Message.ID)
	}

	if _, err := svc.StopConversation(context.Background(), scope, prepared.Conversation.ID); err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, aichatservice.ErrMessageStopped) {
			t.Fatalf("RunPreparedStream() error = %v, want ErrMessageStopped", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("RunPreparedStream() did not return after StopConversation")
	}
}

func TestService_RunPreparedStreamIgnoresChunkDeliveryError(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"hello", " world"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	deliveryErr := errors.New("client disconnected")
	result, err := svc.RunPreparedStream(context.Background(), prepared, func(string) error {
		return deliveryErr
	})
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "hello world" {
		t.Fatalf("answer = %q, want hello world", result.Answer)
	}

	var message aichatmodel.Message
	if err := db.Where("id = ?", prepared.Message.ID).Take(&message).Error; err != nil {
		t.Fatalf("load message: %v", err)
	}
	if message.Status != aichatmodel.MessageStatusCompleted {
		t.Fatalf("message status = %q, want completed", message.Status)
	}
	if message.Error != nil {
		t.Fatalf("message error = %v, want nil", *message.Error)
	}
}

func TestService_PrepareChatDoesNotPersistConversationSkillConfig(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversation := &aichatmodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Skill config",
		Status:         aichatmodel.ConversationStatusNormal,
		RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
		Source:         aichatmodel.ConversationSourceConsole,
		Metadata: map[string]interface{}{
			"existing": "keep",
		},
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}}, nil, functionCallingModelResolver(), nil, nil, nil, newTestSkillRuntime())
	_, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		ConversationID: conversation.ID.String(),
		Query:          "use skills",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	var stored aichatmodel.Conversation
	if err := db.Where("id = ?", conversation.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if stored.Metadata["existing"] != "keep" {
		t.Fatalf("existing metadata = %v, want keep", stored.Metadata["existing"])
	}
	if _, ok := stored.Metadata["skill_config"]; ok {
		t.Fatalf("metadata = %#v, want no skill_config", stored.Metadata)
	}
}

func TestService_PrepareChatUsesDefaultOrganizationSkillConfigForNewConversation(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}}, nil, functionCallingModelResolver(), nil, nil, nil, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "plain chat",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	if got := metadataStringSlice(prepared.Message.Metadata["configured_skill_ids"]); !sameStrings(got, []string{"calculator", "file-generator", "time"}) {
		t.Fatalf("configured_skill_ids = %v, want system skills", got)
	}
	var stored aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if _, ok := stored.Metadata["skill_config"]; ok {
		t.Fatalf("metadata = %#v, want no skill_config", stored.Metadata)
	}
}

func TestService_PrepareChatUsesOrganizationSkillConfig(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	seedOrganizationSkillConfigs(t, db, orgID, map[string]bool{
		"time":           true,
		"calculator":     true,
		"file-generator": false,
	})

	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}}, nil, functionCallingModelResolver(), nil, nil, nil, newTestSkillRuntime())
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "plain chat",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if got := metadataStringSlice(prepared.Message.Metadata["configured_skill_ids"]); !sameStrings(got, []string{"calculator", "time"}) {
		t.Fatalf("configured_skill_ids = %v, want enabled organization skills", got)
	}
}

func TestService_UpdateSkillConfigPersistsOrganizationConfig(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}}, nil, functionCallingModelResolver(), nil, nil, nil, newTestSkillRuntime())
	config, err := svc.UpdateSkillConfig(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.UpdateSkillConfigRequest{
		EnabledSkillIDs: []string{"calculator"},
	})
	if err != nil {
		t.Fatalf("UpdateSkillConfig() error = %v", err)
	}
	if !sameStrings(config.EnabledSkillIDs, []string{"calculator"}) {
		t.Fatalf("enabled skill ids = %v, want calculator", config.EnabledSkillIDs)
	}

	read, err := svc.GetSkillConfig(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID})
	if err != nil {
		t.Fatalf("GetSkillConfig() error = %v", err)
	}
	if !sameStrings(read.EnabledSkillIDs, []string{"calculator"}) {
		t.Fatalf("read enabled skill ids = %v, want calculator", read.EnabledSkillIDs)
	}

	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "calculate",
		Model: "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if got := metadataStringSlice(prepared.Message.Metadata["configured_skill_ids"]); !sameStrings(got, []string{"calculator"}) {
		t.Fatalf("configured_skill_ids = %v, want calculator", got)
	}
}

func TestService_UpdateSkillConfigRejectsUnknownSkill(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewServiceWithSkillRuntime(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}}, nil, nil, nil, nil, nil, newTestSkillRuntime())
	_, err := svc.UpdateSkillConfig(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.UpdateSkillConfigRequest{
		EnabledSkillIDs: []string{"unknown"},
	})
	if !errors.Is(err, aichatservice.ErrInvalidInput) {
		t.Fatalf("UpdateSkillConfig() error = %v, want ErrInvalidInput", err)
	}
}

func TestService_ListConversationsOrdersByLatestActivity(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	base := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	oldConversation := &aichatmodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Old",
		Status:         aichatmodel.ConversationStatusNormal,
		RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      base,
		UpdatedAt:      base,
	}
	newConversation := &aichatmodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "New",
		Status:         aichatmodel.ConversationStatusNormal,
		RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      base.Add(time.Minute),
		UpdatedAt:      base.Add(time.Minute),
	}
	if err := db.Create(oldConversation).Error; err != nil {
		t.Fatalf("create old conversation: %v", err)
	}
	if err := db.Create(newConversation).Error; err != nil {
		t.Fatalf("create new conversation: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	conversations, _, err := svc.ListConversations(context.Background(), scope, 1, 10)
	if err != nil {
		t.Fatalf("ListConversations() error = %v", err)
	}
	if len(conversations) < 2 {
		t.Fatalf("conversation count = %d, want at least 2", len(conversations))
	}
	if conversations[0].ID != newConversation.ID || conversations[1].ID != oldConversation.ID {
		t.Fatalf("initial order = %s, %s; want new, old", conversations[0].ID, conversations[1].ID)
	}

	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: oldConversation.ID.String(),
		Query:          "bring this to top",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	conversations, _, err = svc.ListConversations(context.Background(), scope, 1, 10)
	if err != nil {
		t.Fatalf("ListConversations() after activity error = %v", err)
	}
	if len(conversations) < 2 {
		t.Fatalf("conversation count after activity = %d, want at least 2", len(conversations))
	}
	if conversations[0].ID != oldConversation.ID || conversations[1].ID != newConversation.ID {
		t.Fatalf("order after activity = %s, %s; want old, new", conversations[0].ID, conversations[1].ID)
	}
	if _, err := svc.StopConversation(context.Background(), scope, prepared.Conversation.ID); err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
}

func TestService_RunPreparedStreamDoesNotAdvanceConversationUpdatedAtOnCompletion(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"done"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	activityTime := time.Date(2026, 5, 13, 10, 30, 0, 0, time.UTC)
	if err := db.Model(&aichatmodel.Conversation{}).
		Where("id = ?", prepared.Conversation.ID).
		Update("updated_at", activityTime).Error; err != nil {
		t.Fatalf("set activity time: %v", err)
	}

	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	var completed aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&completed).Error; err != nil {
		t.Fatalf("load completed conversation: %v", err)
	}
	if !completed.UpdatedAt.Equal(activityTime) {
		t.Fatalf("updated_at = %s, want %s", completed.UpdatedAt, activityTime)
	}
}

func TestService_RegenerateOnlyRootMessageReplacesSameMessage(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	rootID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Root replace",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &rootID,
		DialogueCount:        1,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	root := newStoredMessage(rootID, conversationID, nil, "old query", now)
	root.Answer = "old answer"
	if err := db.Create(root).Error; err != nil {
		t.Fatalf("create root message: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"new", " answer"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	newQuery := "new query"
	newModel := "gpt-new"
	prepared, err := svc.PrepareRootRegeneration(context.Background(), scope, rootID, aichatdto.RegenerateMessageRequest{
		Query: &newQuery,
		Model: &newModel,
		Parameters: map[string]interface{}{
			"temperature": 0.4,
		},
	})
	if err != nil {
		t.Fatalf("PrepareRootRegeneration() error = %v", err)
	}
	if !prepared.ReplaceRoot {
		t.Fatalf("ReplaceRoot = false, want true")
	}
	if prepared.Message.ID != rootID {
		t.Fatalf("message id = %s, want %s", prepared.Message.ID, rootID)
	}
	if len(prepared.LLMRequest.Messages) != 2 || prepared.LLMRequest.Messages[1].Content != "new query" {
		t.Fatalf("llm messages = %#v, want system and new query", prepared.LLMRequest.Messages)
	}

	var streamingMessage aichatmodel.Message
	if err := db.Where("id = ?", rootID).Take(&streamingMessage).Error; err != nil {
		t.Fatalf("load streaming message: %v", err)
	}
	if streamingMessage.Status != aichatmodel.MessageStatusStreaming {
		t.Fatalf("status = %q, want streaming", streamingMessage.Status)
	}
	if streamingMessage.Answer != "" {
		t.Fatalf("answer = %q, want empty during replacement", streamingMessage.Answer)
	}
	if streamingMessage.Query != "new query" || streamingMessage.ModelName != "gpt-new" {
		t.Fatalf("message query/model = %q/%q, want new query/gpt-new", streamingMessage.Query, streamingMessage.ModelName)
	}

	result, err := svc.RunPreparedStream(context.Background(), prepared, nil)
	if err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if result.Answer != "new answer" {
		t.Fatalf("answer = %q, want new answer", result.Answer)
	}

	var completedMessage aichatmodel.Message
	if err := db.Where("id = ?", rootID).Take(&completedMessage).Error; err != nil {
		t.Fatalf("load completed message: %v", err)
	}
	if completedMessage.Status != aichatmodel.MessageStatusCompleted || completedMessage.Answer != "new answer" {
		t.Fatalf("completed message = status %q answer %q, want completed/new answer", completedMessage.Status, completedMessage.Answer)
	}
	var completedConversation aichatmodel.Conversation
	if err := db.Where("id = ?", conversationID).Take(&completedConversation).Error; err != nil {
		t.Fatalf("load completed conversation: %v", err)
	}
	if completedConversation.DialogueCount != 1 {
		t.Fatalf("dialogue_count = %d, want 1", completedConversation.DialogueCount)
	}
	if completedConversation.CurrentLeafMessageID == nil || *completedConversation.CurrentLeafMessageID != rootID {
		t.Fatalf("current_leaf_message_id = %v, want %s", completedConversation.CurrentLeafMessageID, rootID)
	}
	if completedConversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle || completedConversation.ActiveMessageID != nil {
		t.Fatalf("conversation runtime = %q active=%v, want idle nil", completedConversation.RuntimeStatus, completedConversation.ActiveMessageID)
	}
	var count int64
	if err := db.Model(&aichatmodel.Message{}).Where("conversation_id = ? AND deleted_at IS NULL", conversationID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("message count = %d, want 1", count)
	}
}

func TestService_RegenerateRootMessageResetsOldStreamEvents(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	withRedisClient(t)

	fakeLLM := &fakeLLMClient{chunks: []string{"old answer"}}
	svc := aichatservice.NewService(repository.NewRepositories(db), fakeLLM)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "root", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() initial error = %v", err)
	}

	fakeLLM.chunks = []string{"new answer"}
	replacement, err := svc.PrepareRootRegeneration(context.Background(), scope, prepared.Message.ID, aichatdto.RegenerateMessageRequest{})
	if err != nil {
		t.Fatalf("PrepareRootRegeneration() error = %v", err)
	}
	if _, err := svc.RunPreparedStream(context.Background(), replacement, nil); err != nil {
		t.Fatalf("RunPreparedStream() replacement error = %v", err)
	}

	var events []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, prepared.Conversation.ID, prepared.Message.ID, "0", func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3: %#v", len(events), events)
	}
	if events[1].Payload["answer"] != "new answer" {
		t.Fatalf("message event answer = %#v, want new answer", events[1].Payload["answer"])
	}
	if events[0].Payload["replace"] != true {
		t.Fatalf("message_start replace = %#v, want true", events[0].Payload["replace"])
	}
}

func TestService_RegenerateRootMessageRejectsConversationWithChildren(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	rootID := uuid.New()
	childID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Root with child",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &childID,
		DialogueCount:        2,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(newStoredMessage(rootID, conversationID, nil, "root", now)).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(newStoredMessage(childID, conversationID, &rootID, "child", now.Add(time.Second))).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	_, err := svc.PrepareRootRegeneration(context.Background(), scope, rootID, aichatdto.RegenerateMessageRequest{})
	if !errors.Is(err, aichatservice.ErrMessageReplaceNotAllowed) {
		t.Fatalf("PrepareRootRegeneration(root) error = %v, want ErrMessageReplaceNotAllowed", err)
	}
	_, err = svc.PrepareRootRegeneration(context.Background(), scope, childID, aichatdto.RegenerateMessageRequest{})
	if !errors.Is(err, aichatservice.ErrMessageReplaceNotAllowed) {
		t.Fatalf("PrepareRootRegeneration(child) error = %v, want ErrMessageReplaceNotAllowed", err)
	}
}

func TestService_ChatRejectsNonMember(t *testing.T) {
	db := openAIChatTestDB(t)
	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})

	_, err := svc.PrepareChat(context.Background(), aichatservice.Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if !errors.Is(err, aichatservice.ErrPermissionDenied) {
		t.Fatalf("PrepareChat() error = %v, want ErrPermissionDenied", err)
	}
}

func TestService_StreamErrorMarksMessageError(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{streamErr: errors.New("model failed")})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	_, err = svc.RunPreparedStream(context.Background(), prepared, nil)
	if err == nil {
		t.Fatalf("RunPreparedStream() error = nil, want error")
	}

	var stored aichatmodel.Message
	if err := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load stored message: %v", err)
	}
	if stored.Status != aichatmodel.MessageStatusError {
		t.Fatalf("status = %q, want error", stored.Status)
	}
	if stored.Error == nil || *stored.Error != "model failed" {
		t.Fatalf("stored error = %v, want model failed", stored.Error)
	}
	var conversation aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&conversation).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if conversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", conversation.RuntimeStatus)
	}
	if conversation.ActiveMessageID != nil {
		t.Fatalf("active_message_id = %v, want nil", conversation.ActiveMessageID)
	}
}

func TestService_ChatContextKeepsLatestTwentyTurnsInChronologicalOrder(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Long context",
		Status:         aichatmodel.ConversationStatusNormal,
		DialogueCount:  25,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	var parentID *uuid.UUID
	for i := 1; i <= 25; i++ {
		id := uuid.New()
		message := newStoredMessage(id, conversationID, parentID, formatTurnQuery(i), now.Add(time.Duration(i)*time.Second))
		message.Answer = formatTurnAnswer(i)
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		value := id
		parentID = &value
	}
	if err := db.Model(&aichatmodel.Conversation{}).Where("id = ?", conversationID).Update("current_leaf_message_id", parentID).Error; err != nil {
		t.Fatalf("update current leaf: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: conversationID.String(),
		Query:          "current query",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	messages := prepared.LLMRequest.Messages
	if len(messages) != 42 {
		t.Fatalf("llm message count = %d, want 42", len(messages))
	}
	if messages[0].Role != "system" {
		t.Fatalf("first role = %q, want system", messages[0].Role)
	}
	if messages[1].Role != "user" || messages[1].Content != "turn-06" {
		t.Fatalf("first history user = %#v, want turn-06", messages[1])
	}
	if messages[2].Role != "assistant" || messages[2].Content != "answer-06" {
		t.Fatalf("first history assistant = %#v, want answer-06", messages[2])
	}
	if messages[39].Role != "user" || messages[39].Content != "turn-25" {
		t.Fatalf("last history user = %#v, want turn-25", messages[39])
	}
	if messages[40].Role != "assistant" || messages[40].Content != "answer-25" {
		t.Fatalf("last history assistant = %#v, want answer-25", messages[40])
	}
	if messages[41].Role != "user" || messages[41].Content != "current query" {
		t.Fatalf("current query message = %#v", messages[41])
	}
}

func TestService_ChatContextKeepsAllTurnsBelowTwentyLimit(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Short context",
		Status:         aichatmodel.ConversationStatusNormal,
		DialogueCount:  3,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	var parentID *uuid.UUID
	for i := 1; i <= 3; i++ {
		id := uuid.New()
		message := newStoredMessage(id, conversationID, parentID, formatTurnQuery(i), now.Add(time.Duration(i)*time.Second))
		message.Answer = formatTurnAnswer(i)
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		value := id
		parentID = &value
	}
	if err := db.Model(&aichatmodel.Conversation{}).Where("id = ?", conversationID).Update("current_leaf_message_id", parentID).Error; err != nil {
		t.Fatalf("update current leaf: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: conversationID.String(),
		Query:          "current query",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	messages := prepared.LLMRequest.Messages
	if len(messages) != 8 {
		t.Fatalf("llm message count = %d, want 8", len(messages))
	}
	if messages[1].Content != "turn-01" || messages[5].Content != "turn-03" {
		t.Fatalf("history ordering = %#v", messages)
	}
}

func TestService_ChatContextIncludesStoppedAssistantAnswer(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Stopped context",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &messageID,
		DialogueCount:        1,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message := newStoredMessage(messageID, conversationID, nil, "interrupted", now)
	message.Status = aichatmodel.MessageStatusStopped
	message.Answer = "partial answer"
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("create stopped message: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		ConversationID: conversationID.String(),
		Query:          "continue",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	messages := prepared.LLMRequest.Messages
	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(messages))
	}
	if messages[2].Role != "assistant" || messages[2].Content != "partial answer" {
		t.Fatalf("stopped assistant history = %#v, want partial answer", messages[2])
	}
}

func TestService_ChatContextUsesTokenBudgetAndTruncatesOldHistory(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Token budget",
		Status:         aichatmodel.ConversationStatusNormal,
		DialogueCount:  6,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	var parentID *uuid.UUID
	for i := 1; i <= 6; i++ {
		id := uuid.New()
		message := newStoredMessage(id, conversationID, parentID, fmt.Sprintf("turn-%02d %s", i, strings.Repeat("u", 260)), now.Add(time.Duration(i)*time.Second))
		message.Answer = fmt.Sprintf("answer-%02d %s", i, strings.Repeat("a", 260))
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		value := id
		parentID = &value
	}
	if err := db.Model(&aichatmodel.Conversation{}).Where("id = ?", conversationID).Update("current_leaf_message_id", parentID).Error; err != nil {
		t.Fatalf("update current leaf: %v", err)
	}

	svc := aichatservice.NewServiceWithTitleGeneratorAndModelSpecResolver(
		repository.NewRepositories(db),
		&fakeLLMClient{},
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 1200}},
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		ConversationID: conversationID.String(),
		Query:          "current query",
		Model:          "custom-small",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	messages := prepared.LLMRequest.Messages
	if messages[0].Role != "system" || messages[len(messages)-1].Content != "current query" {
		t.Fatalf("messages should keep system and current query: %#v", messages)
	}
	if len(messages) >= 14 {
		t.Fatalf("message count = %d, want truncated below full history", len(messages))
	}
	if !containsMessageContentPrefix(messages, "turn-06") || containsMessageContentPrefix(messages, "turn-01") {
		t.Fatalf("history selection = %#v, want recent history without oldest turn", messages)
	}

	contextControl := prepared.Message.Metadata["context_control"].(map[string]interface{})
	if contextControl["strategy"] != "token_budget" {
		t.Fatalf("context strategy = %#v, want token_budget", contextControl["strategy"])
	}
	if contextControl["truncated"] != true {
		t.Fatalf("truncated = %#v, want true", contextControl["truncated"])
	}
	if contextControl["history_messages_before"] == contextControl["history_messages_after"] {
		t.Fatalf("history before/after = %#v, want reduced", contextControl)
	}
}

func TestService_ChatContextDoesNotReserveEntireMaxOutputWindow(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewServiceWithTitleGeneratorAndModelSpecResolver(
		repository.NewRepositories(db),
		&fakeLLMClient{},
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 8192, MaxOutputTokens: 8192}},
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "hello",
		Model: "custom-same-window",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	contextControl := prepared.Message.Metadata["context_control"].(map[string]interface{})
	if metadataInt(contextControl["reserved_output_tokens"]) != 1024 {
		t.Fatalf("reserved output = %#v, want default 1024", contextControl["reserved_output_tokens"])
	}
	if metadataInt(contextControl["prompt_budget"]) <= 0 {
		t.Fatalf("prompt budget = %#v, want positive", contextControl["prompt_budget"])
	}
}

func TestService_ChatContextClampsOversizedMaxTokens(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewServiceWithTitleGeneratorAndModelSpecResolver(
		repository.NewRepositories(db),
		&fakeLLMClient{},
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 4096, MaxOutputTokens: 4096}},
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: "hello",
		Model: "custom-small",
		Parameters: map[string]interface{}{
			"max_tokens": 3000,
		},
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if prepared.LLMRequest.MaxTokens == nil || *prepared.LLMRequest.MaxTokens != 2662 {
		t.Fatalf("MaxTokens = %v, want 2662", prepared.LLMRequest.MaxTokens)
	}
	contextControl := prepared.Message.Metadata["context_control"].(map[string]interface{})
	if contextControl["max_tokens_clamped"] != true {
		t.Fatalf("max_tokens_clamped = %#v, want true", contextControl["max_tokens_clamped"])
	}
	if metadataInt(contextControl["original_max_tokens"]) != 3000 || metadataInt(contextControl["effective_max_tokens"]) != 2662 {
		t.Fatalf("max token metadata = %#v, want 3000 -> 2662", contextControl)
	}
}

func TestService_ChatContextRejectsBasePromptOverBudget(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	svc := aichatservice.NewServiceWithTitleGeneratorAndModelSpecResolver(
		repository.NewRepositories(db),
		&fakeLLMClient{},
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 1200}},
	)
	_, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query: strings.Repeat("x", 5000),
		Model: "custom-small",
	})
	if !errors.Is(err, aichatservice.ErrInvalidInput) {
		t.Fatalf("PrepareChat() error = %v, want ErrInvalidInput", err)
	}
}

func TestService_ChatContextSkipsUnusableAssistantHistory(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	streamingID := uuid.New()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Assistant status",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &streamingID,
		DialogueCount:        3,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	completedID := uuid.New()
	stoppedID := uuid.New()
	errorID := uuid.New()
	completed := newStoredMessage(completedID, conversationID, nil, "completed query", now)
	completed.Answer = "completed answer"
	stopped := newStoredMessage(stoppedID, conversationID, &completedID, "stopped query", now.Add(time.Second))
	stopped.Status = aichatmodel.MessageStatusStopped
	stopped.Answer = "stopped answer"
	failed := newStoredMessage(errorID, conversationID, &stoppedID, "failed query", now.Add(2*time.Second))
	failed.Status = aichatmodel.MessageStatusError
	failed.Answer = "failed answer"
	streaming := newStoredMessage(streamingID, conversationID, &errorID, "streaming query", now.Add(3*time.Second))
	streaming.Status = aichatmodel.MessageStatusStreaming
	streaming.Answer = "streaming answer"
	for _, message := range []*aichatmodel.Message{completed, stopped, failed, streaming} {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	svc := aichatservice.NewServiceWithTitleGeneratorAndModelSpecResolver(
		repository.NewRepositories(db),
		&fakeLLMClient{},
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 8192}},
	)
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		ConversationID: conversationID.String(),
		Query:          "current query",
		Model:          "custom-status",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	messages := prepared.LLMRequest.Messages
	if !containsExactAssistantContent(messages, "completed answer") || !containsExactAssistantContent(messages, "stopped answer") {
		t.Fatalf("messages = %#v, want completed and stopped assistant answers", messages)
	}
	if containsExactAssistantContent(messages, "failed answer") || containsExactAssistantContent(messages, "streaming answer") {
		t.Fatalf("messages = %#v, want failed/streaming assistant answers excluded", messages)
	}
}

func TestService_ChatWithFilesIncludesAttachmentContextAndMetadata(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fileID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			fileID: newTestUploadFile(fileID, orgID.String(), accountID.String(), nil, false),
		},
	}
	extractor := &fakeAIChatContentExtractor{
		contents: map[string]string{fileID: "Quarterly revenue notes"},
	}
	fakeLLM := &fakeLLMClient{chunks: []string{"ok"}}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		nil,
		fileSvc,
		extractor,
		&fakeWorkspacePermissionService{},
	)

	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "Summarize this",
		FileIDs: []string{fileID},
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if prepared.Message.Query != "Summarize this" {
		t.Fatalf("stored query = %q, want raw query", prepared.Message.Query)
	}
	var events []aichatservice.StreamEvent
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("llm request count = %d, want 1", len(fakeLLM.requests))
	}
	current := fakeLLM.requests[0].Messages[len(fakeLLM.requests[0].Messages)-1]
	content, _ := current.Content.(string)
	if !strings.Contains(content, "Summarize this") || !strings.Contains(content, "Quarterly revenue notes") {
		t.Fatalf("current user content = %q, want query and attachment content", content)
	}
	if len(events) != 2 || events[0].EventType != "file_parse_start" || events[1].EventType != "file_parse_end" {
		t.Fatalf("events = %#v, want file_parse_start/end", events)
	}
	files := metadataFilesFromValue(prepared.Message.Metadata["files"])
	if len(files) != 1 {
		t.Fatalf("metadata files length = %d, want 1", len(files))
	}
	if files[0]["id"] != fileID || files[0]["content_preview"] != "Quarterly revenue notes" {
		t.Fatalf("metadata files = %#v, want file snapshot", files)
	}
}

func TestService_ChatWithVisionModelSendsImageContentPart(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	imageID := uuid.NewString()
	imageFile := newTestUploadFile(imageID, orgID.String(), accountID.String(), nil, false)
	imageFile.Name = "chart.png"
	imageFile.Extension = "png"
	imageFile.MimeType = "image/png"
	imageURL := "https://files.example.com/chart.png"
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{imageID: imageFile},
		urls:  map[string]string{imageID: imageURL},
	}
	fakeLLM := &fakeLLMClient{chunks: []string{"vision ok"}}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 8192, UseCases: []string{"text-chat", "vision"}}},
		fileSvc,
		&fakeAIChatContentExtractor{},
		&fakeWorkspacePermissionService{},
	)

	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "Read this chart",
		FileIDs: []string{imageID},
		Model:   "gpt-vision",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	var events []aichatservice.StreamEvent
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("llm request count = %d, want 1", len(fakeLLM.requests))
	}
	current := fakeLLM.requests[0].Messages[len(fakeLLM.requests[0].Messages)-1]
	parts, ok := current.Content.([]adapter.MessageContentPart)
	if !ok {
		t.Fatalf("current user content = %#v, want multimodal parts", current.Content)
	}
	if len(parts) != 2 || parts[0].Type != "image_url" || parts[0].ImageURL == nil || parts[0].ImageURL.URL != imageURL {
		t.Fatalf("image parts = %#v, want first image_url with signed URL", parts)
	}
	if parts[1].Type != "text" || !strings.Contains(parts[1].Text, "Read this chart") {
		t.Fatalf("text part = %#v, want query text", parts[1])
	}
	if got := eventTypes(events); strings.Join(got, ",") != "file_parse_start,file_parse_end" {
		t.Fatalf("events = %#v, want file parse start/end", got)
	}
	if events[1].Payload["content_status"] != "vision_ready" || events[1].Payload["vision_detail"] != "high" {
		t.Fatalf("file_parse_end payload = %#v, want vision_ready high", events[1].Payload)
	}
	files := metadataFilesFromValue(prepared.Message.Metadata["files"])
	if len(files) != 1 || files[0]["kind"] != "image" || files[0]["content_status"] != "vision_ready" {
		t.Fatalf("metadata files = %#v, want vision image snapshot", files)
	}
}

func TestService_ChatWithNonVisionModelFiltersImagesAndKeepsDocuments(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	imageID := uuid.NewString()
	imageFile := newTestUploadFile(imageID, orgID.String(), accountID.String(), nil, false)
	imageFile.Name = "photo.jpg"
	imageFile.Extension = "jpg"
	imageFile.MimeType = "image/jpeg"
	docID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			imageID: imageFile,
			docID:   newTestUploadFile(docID, orgID.String(), accountID.String(), nil, false),
		},
		urls: map[string]string{imageID: "https://files.example.com/photo.jpg"},
	}
	fakeLLM := &fakeLLMClient{chunks: []string{"ok"}}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 8192, UseCases: []string{"text-chat"}}},
		fileSvc,
		&fakeAIChatContentExtractor{
			contents: map[string]string{docID: "document notes"},
			errors:   map[string]error{imageID: errors.New("image should not be extracted")},
		},
		&fakeWorkspacePermissionService{},
	)

	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "Summarize attachments",
		FileIDs: []string{imageID, docID},
		Model:   "gpt-text",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	var events []aichatservice.StreamEvent
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("llm request count = %d, want 1", len(fakeLLM.requests))
	}
	current := fakeLLM.requests[0].Messages[len(fakeLLM.requests[0].Messages)-1]
	content, ok := current.Content.(string)
	if !ok {
		t.Fatalf("current user content = %#v, want text-only content", current.Content)
	}
	if !strings.Contains(content, "Summarize attachments") || !strings.Contains(content, "document notes") {
		t.Fatalf("current user content = %q, want query and document text", content)
	}
	if strings.Contains(content, "photo.jpg") {
		t.Fatalf("current user content = %q, filtered image should not enter prompt text", content)
	}
	if got := eventTypes(events); strings.Join(got, ",") != "file_parse_start,file_parse_end,file_parse_start,file_parse_end" {
		t.Fatalf("events = %#v, want parse start/end for image and document", got)
	}
	if events[1].Payload["content_status"] != "filtered" || events[1].Payload["filtered_reason"] != "model_without_vision" {
		t.Fatalf("image parse end payload = %#v, want filtered image", events[1].Payload)
	}
	files := metadataFilesFromValue(prepared.Message.Metadata["files"])
	if len(files) != 2 {
		t.Fatalf("metadata files length = %d, want 2", len(files))
	}
	if files[0]["kind"] != "image" || files[0]["content_status"] != "filtered" || files[0]["filtered_reason"] != "model_without_vision" {
		t.Fatalf("image metadata = %#v, want filtered image", files[0])
	}
	if files[1]["kind"] != "document" || files[1]["content_preview"] != "document notes" {
		t.Fatalf("document metadata = %#v, want extracted document", files[1])
	}
}

func TestService_ChatWithTemporaryFileRequiresCreator(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fileID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			fileID: newTestUploadFile(fileID, uuid.NewString(), otherAccountID.String(), nil, true),
		},
	}
	fakeLLM := &fakeLLMClient{chunks: []string{"done"}}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		nil,
		fileSvc,
		&fakeAIChatContentExtractor{},
		&fakeWorkspacePermissionService{},
	)

	_, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "read file",
		FileIDs: []string{fileID},
		Model:   "gpt-test",
	})
	if !errors.Is(err, aichatservice.ErrPermissionDenied) {
		t.Fatalf("PrepareChat() error = %v, want ErrPermissionDenied", err)
	}
	var count int64
	if err := db.Model(&aichatmodel.Message{}).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("message count = %d, want 0", count)
	}
}

func TestService_ChatWithWorkspaceFileUsesDownloadPermission(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	workspaceID := uuid.NewString()
	seedMember(t, db, orgID, accountID)

	fileID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			fileID: newTestUploadFile(fileID, orgID.String(), otherAccountID.String(), &workspaceID, false),
		},
	}
	extractor := &fakeAIChatContentExtractor{contents: map[string]string{fileID: "workspace file text"}}
	perms := &fakeWorkspacePermissionService{allowed: true}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		&fakeLLMClient{},
		nil,
		nil,
		fileSvc,
		extractor,
		perms,
	)

	if _, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "read workspace file",
		FileIDs: []string{fileID},
		Model:   "gpt-test",
	}); err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if perms.lastWorkspaceID != workspaceID || perms.lastPermission != workspacemodel.WorkspacePermissionFileDownload {
		t.Fatalf("permission check = workspace %q permission %q, want file.download", perms.lastWorkspaceID, perms.lastPermission)
	}
}

func TestService_ChatWithLargeAttachmentTruncatesWithinTokenBudget(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fileID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			fileID: newTestUploadFile(fileID, orgID.String(), accountID.String(), nil, false),
		},
	}
	largeContent := strings.Repeat("attachment words ", 3000) + "TAIL_SHOULD_NOT_APPEAR"
	fakeLLM := &fakeLLMClient{chunks: []string{"done"}}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		&fakeModelSpecResolver{spec: aichatservice.ModelSpec{ContextWindow: 4096}},
		fileSvc,
		&fakeAIChatContentExtractor{contents: map[string]string{fileID: largeContent}},
		&fakeWorkspacePermissionService{},
	)

	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "Keep this query",
		FileIDs: []string{fileID},
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}
	if len(fakeLLM.requests) != 1 {
		t.Fatalf("llm request count = %d, want 1", len(fakeLLM.requests))
	}
	current := fakeLLM.requests[0].Messages[len(fakeLLM.requests[0].Messages)-1]
	content, _ := current.Content.(string)
	if !strings.Contains(content, "Keep this query") {
		t.Fatalf("current user content = %q, want query preserved", content)
	}
	if strings.Contains(content, "TAIL_SHOULD_NOT_APPEAR") {
		t.Fatalf("current user content contains untruncated attachment tail")
	}
	control, ok := prepared.Message.Metadata["context_control"].(map[string]interface{})
	if !ok {
		t.Fatalf("context_control = %#v, want map", prepared.Message.Metadata["context_control"])
	}
	if truncated, _ := control["attachments_truncated"].(bool); !truncated {
		t.Fatalf("attachments_truncated = %#v, want true", control["attachments_truncated"])
	}
}

func TestService_ChatFileParseFailureMarksMessageErrorAndSkipsLLM(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	fileID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			fileID: newTestUploadFile(fileID, orgID.String(), accountID.String(), nil, false),
		},
	}
	fakeLLM := &fakeLLMClient{chunks: []string{"should not run"}}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		fakeLLM,
		nil,
		nil,
		fileSvc,
		&fakeAIChatContentExtractor{errors: map[string]error{fileID: errors.New("parse failed")}},
		&fakeWorkspacePermissionService{},
	)

	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		Query:   "read file",
		FileIDs: []string{fileID},
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	var events []aichatservice.StreamEvent
	_, err = svc.RunPreparedStream(context.Background(), prepared, nil, func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if !errors.Is(err, aichatservice.ErrInvalidInput) {
		t.Fatalf("RunPreparedStream() error = %v, want ErrInvalidInput", err)
	}
	if len(fakeLLM.requests) != 0 {
		t.Fatalf("llm request count = %d, want 0", len(fakeLLM.requests))
	}
	if got := eventTypes(events); strings.Join(got, ",") != "file_parse_start,file_parse_error" {
		t.Fatalf("events = %#v, want file parse start/error", got)
	}

	var stored aichatmodel.Message
	if err := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load message: %v", err)
	}
	if stored.Status != aichatmodel.MessageStatusError {
		t.Fatalf("message status = %q, want error", stored.Status)
	}
	var conversation aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&conversation).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if conversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle || conversation.ActiveMessageID != nil {
		t.Fatalf("conversation runtime = %q active=%v, want idle nil", conversation.RuntimeStatus, conversation.ActiveMessageID)
	}
}

func TestService_StreamConversationEventsReplaysFileParseEvents(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	withRedisClient(t)

	fileID := uuid.NewString()
	fileSvc := &fakeAIChatFileService{
		limit: 10,
		files: map[string]*sharedto.UploadFile{
			fileID: newTestUploadFile(fileID, orgID.String(), accountID.String(), nil, false),
		},
	}
	svc := aichatservice.NewServiceWithDependencies(
		repository.NewRepositories(db),
		&fakeLLMClient{chunks: []string{"answer"}},
		nil,
		nil,
		fileSvc,
		&fakeAIChatContentExtractor{
			contents:  map[string]string{fileID: "cached content"},
			fromCache: map[string]bool{fileID: true},
		},
		&fakeWorkspacePermissionService{},
	)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{
		Query:   "read file",
		FileIDs: []string{fileID},
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}

	var replayed []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, prepared.Conversation.ID, prepared.Message.ID, "0", func(event aichatservice.StreamEvent) error {
		replayed = append(replayed, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() error = %v", err)
	}
	if got := eventTypes(replayed); strings.Join(got, ",") != "message_start,file_parse_start,file_parse_end,message,message_end" {
		t.Fatalf("event types = %#v", got)
	}
	parseEnd := replayed[2].Payload
	if parseEnd["from_cache"] != true || parseEnd["content_chars"] == nil {
		t.Fatalf("file_parse_end payload = %#v, want from_cache and content_chars", parseEnd)
	}
}

func TestService_ChatHistoryIncludesAttachmentPreview(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	rootID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Attachment history",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &rootID,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	root := newStoredMessage(rootID, conversationID, nil, "previous query", now)
	root.Metadata = map[string]interface{}{
		"files": []map[string]interface{}{{
			"id":              uuid.NewString(),
			"name":            "history.txt",
			"content_preview": "historical preview text",
			"content_status":  "extracted",
		}},
	}
	if err := db.Create(root).Error; err != nil {
		t.Fatalf("create root message: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	prepared, err := svc.PrepareChat(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, aichatdto.ChatRequest{
		ConversationID: conversationID.String(),
		Query:          "continue",
		Model:          "gpt-test",
	})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if !containsUserContent(prepared.LLMRequest.Messages, "historical preview text") {
		t.Fatalf("messages = %#v, want historical attachment preview", prepared.LLMRequest.Messages)
	}
}

func TestService_StopMessageCancelsActiveStream(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	started := make(chan struct{})
	fakeLLM := &fakeLLMClient{blockUntilCancel: true, started: started}
	svc := aichatservice.NewService(repository.NewRepositories(db), fakeLLM)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.RunPreparedStream(context.Background(), prepared, nil)
		errCh <- err
	}()

	<-started
	stopped, err := svc.StopMessage(context.Background(), scope, prepared.Message.ID)
	if err != nil {
		t.Fatalf("StopMessage() error = %v", err)
	}
	if stopped.Status != aichatmodel.MessageStatusStopped {
		t.Fatalf("stopped status = %q, want stopped", stopped.Status)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, aichatservice.ErrMessageStopped) {
			t.Fatalf("RunPreparedStream() error = %v, want ErrMessageStopped", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("RunPreparedStream() did not return after StopMessage")
	}

	var stored aichatmodel.Message
	if err := db.Where("id = ?", prepared.Message.ID).Take(&stored).Error; err != nil {
		t.Fatalf("load stored message: %v", err)
	}
	if stored.Status != aichatmodel.MessageStatusStopped {
		t.Fatalf("stored status = %q, want stopped", stored.Status)
	}
	var conversation aichatmodel.Conversation
	if err := db.Where("id = ?", prepared.Conversation.ID).Take(&conversation).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if conversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", conversation.RuntimeStatus)
	}
	if conversation.ActiveMessageID != nil {
		t.Fatalf("active_message_id = %v, want nil", conversation.ActiveMessageID)
	}
	if conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != prepared.Message.ID {
		t.Fatalf("current_leaf_message_id = %v, want %s", conversation.CurrentLeafMessageID, prepared.Message.ID)
	}
}

func TestService_StopConversationStopsActiveMessage(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	started := make(chan struct{})
	fakeLLM := &fakeLLMClient{blockUntilCancel: true, started: started}
	svc := aichatservice.NewService(repository.NewRepositories(db), fakeLLM)
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.RunPreparedStream(context.Background(), prepared, nil)
		errCh <- err
	}()

	<-started
	result, err := svc.StopConversation(context.Background(), scope, prepared.Conversation.ID)
	if err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	if result.Message == nil || result.Message.ID != prepared.Message.ID {
		t.Fatalf("stopped message = %#v, want %s", result.Message, prepared.Message.ID)
	}
	if result.Message.Status != aichatmodel.MessageStatusStopped {
		t.Fatalf("message status = %q, want stopped", result.Message.Status)
	}
	if result.Conversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", result.Conversation.RuntimeStatus)
	}
	if result.Conversation.ActiveMessageID != nil {
		t.Fatalf("active_message_id = %v, want nil", result.Conversation.ActiveMessageID)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, aichatservice.ErrMessageStopped) {
			t.Fatalf("RunPreparedStream() error = %v, want ErrMessageStopped", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("RunPreparedStream() did not return after StopConversation")
	}
}

func TestService_StopConversationIsIdempotentWhenIdle(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Idle",
		Status:         aichatmodel.ConversationStatusNormal,
		RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	result, err := svc.StopConversation(context.Background(), aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}, conversationID)
	if err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	if result.Message != nil {
		t.Fatalf("message = %#v, want nil", result.Message)
	}
	if result.Conversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", result.Conversation.RuntimeStatus)
	}
}

func TestService_DeleteMessageDeletesDownstreamBranchAndRefreshesLeaf(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	rootID := uuid.New()
	linearLeafID := uuid.New()
	branchID := uuid.New()
	branchLeafID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Branch test",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &branchLeafID,
		DialogueCount:        4,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	messages := []*aichatmodel.Message{
		newStoredMessage(rootID, conversationID, nil, "root", now.Add(time.Second)),
		newStoredMessage(linearLeafID, conversationID, &rootID, "linear", now.Add(2*time.Second)),
		newStoredMessage(branchID, conversationID, &rootID, "branch", now.Add(3*time.Second)),
		newStoredMessage(branchLeafID, conversationID, &branchID, "branch leaf", now.Add(4*time.Second)),
	}
	for _, message := range messages {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	if err := svc.DeleteMessage(context.Background(), scope, branchID); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}

	var remainingCount int64
	if err := db.Model(&aichatmodel.Message{}).Where("conversation_id = ? AND deleted_at IS NULL", conversationID).Count(&remainingCount).Error; err != nil {
		t.Fatalf("count remaining messages: %v", err)
	}
	if remainingCount != 2 {
		t.Fatalf("remaining count = %d, want 2", remainingCount)
	}

	var deletedCount int64
	if err := db.Model(&aichatmodel.Message{}).Where("id IN ? AND deleted_at IS NOT NULL", []uuid.UUID{branchID, branchLeafID}).Count(&deletedCount).Error; err != nil {
		t.Fatalf("count deleted messages: %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("deleted count = %d, want 2", deletedCount)
	}

	var updated aichatmodel.Conversation
	if err := db.Where("id = ?", conversationID).Take(&updated).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if updated.CurrentLeafMessageID == nil || *updated.CurrentLeafMessageID != linearLeafID {
		t.Fatalf("current leaf = %v, want %s", updated.CurrentLeafMessageID, linearLeafID)
	}
	if updated.DialogueCount != 2 {
		t.Fatalf("dialogue count = %d, want 2", updated.DialogueCount)
	}
}

func TestService_DeleteLastMessageClearsConversationLeaf(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Single message",
		Status:               aichatmodel.ConversationStatusNormal,
		CurrentLeafMessageID: &messageID,
		DialogueCount:        1,
		Source:               aichatmodel.ConversationSourceConsole,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(newStoredMessage(messageID, conversationID, nil, "only", now)).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	if err := svc.DeleteMessage(context.Background(), scope, messageID); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}

	var updated aichatmodel.Conversation
	if err := db.Where("id = ?", conversationID).Take(&updated).Error; err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if updated.CurrentLeafMessageID != nil {
		t.Fatalf("current leaf = %v, want nil", updated.CurrentLeafMessageID)
	}
	if updated.DialogueCount != 0 {
		t.Fatalf("dialogue count = %d, want 0", updated.DialogueCount)
	}
}

func TestService_ListMessagesReturnsNewestFirstWithPagination(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Paged messages",
		Status:         aichatmodel.ConversationStatusNormal,
		DialogueCount:  205,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	for i := 1; i <= 205; i++ {
		id := uuid.New()
		message := newStoredMessage(id, conversationID, nil, formatTurnQuery(i), now.Add(time.Duration(i)*time.Second))
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	firstPage, total, err := svc.ListMessages(context.Background(), scope, conversationID, 1, 0)
	if err != nil {
		t.Fatalf("ListMessages() first page error = %v", err)
	}
	if total != 205 {
		t.Fatalf("total = %d, want 205", total)
	}
	if len(firstPage) != 50 {
		t.Fatalf("first page length = %d, want default 50", len(firstPage))
	}
	if firstPage[0].Query != "turn-205" || firstPage[49].Query != "turn-156" {
		t.Fatalf("first page order = first %q last %q, want turn-205..turn-156", firstPage[0].Query, firstPage[49].Query)
	}

	secondPage, _, err := svc.ListMessages(context.Background(), scope, conversationID, 2, 50)
	if err != nil {
		t.Fatalf("ListMessages() second page error = %v", err)
	}
	if len(secondPage) != 50 {
		t.Fatalf("second page length = %d, want 50", len(secondPage))
	}
	if secondPage[0].Query != "turn-155" || secondPage[49].Query != "turn-106" {
		t.Fatalf("second page order = first %q last %q, want turn-155..turn-106", secondPage[0].Query, secondPage[49].Query)
	}

	clamped, _, err := svc.ListMessages(context.Background(), scope, conversationID, 1, 999)
	if err != nil {
		t.Fatalf("ListMessages() clamped page error = %v", err)
	}
	if len(clamped) != 200 {
		t.Fatalf("clamped length = %d, want 200", len(clamped))
	}
}

func TestService_CleanupStaleActiveMessagesMarksOnlyExpiredActiveMessages(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)

	conversationID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Title:          "Cleanup test",
		Status:         aichatmodel.ConversationStatusNormal,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	oldPendingID := uuid.New()
	oldStreamingID := uuid.New()
	freshStreamingID := uuid.New()
	completedID := uuid.New()
	conversation.ActiveMessageID = &oldStreamingID
	conversation.RuntimeStatus = aichatmodel.ConversationRuntimeStatusStreaming
	if err := db.Save(conversation).Error; err != nil {
		t.Fatalf("update active conversation: %v", err)
	}
	messages := []*aichatmodel.Message{
		newStoredMessage(oldPendingID, conversationID, nil, "old pending", now.Add(-2*time.Hour)),
		newStoredMessage(oldStreamingID, conversationID, nil, "old streaming", now.Add(-90*time.Minute)),
		newStoredMessage(freshStreamingID, conversationID, nil, "fresh streaming", now.Add(-30*time.Minute)),
		newStoredMessage(completedID, conversationID, nil, "completed", now.Add(-2*time.Hour)),
	}
	messages[0].Status = aichatmodel.MessageStatusPending
	messages[1].Status = aichatmodel.MessageStatusStreaming
	messages[2].Status = aichatmodel.MessageStatusStreaming
	for _, message := range messages {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	affected, err := svc.CleanupStaleActiveMessages(context.Background())
	if err != nil {
		t.Fatalf("CleanupStaleActiveMessages() error = %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}

	var stored []aichatmodel.Message
	if err := db.Where("conversation_id = ?", conversationID).Find(&stored).Error; err != nil {
		t.Fatalf("list messages: %v", err)
	}
	statuses := map[uuid.UUID]aichatmodel.Message{}
	for _, message := range stored {
		statuses[message.ID] = message
	}
	for _, id := range []uuid.UUID{oldPendingID, oldStreamingID} {
		message := statuses[id]
		if message.Status != aichatmodel.MessageStatusError {
			t.Fatalf("message %s status = %q, want error", id, message.Status)
		}
		if message.Error == nil || *message.Error != "stream interrupted before completion" {
			t.Fatalf("message %s error = %v, want interrupted error", id, message.Error)
		}
	}
	if statuses[freshStreamingID].Status != aichatmodel.MessageStatusStreaming {
		t.Fatalf("fresh status = %q, want streaming", statuses[freshStreamingID].Status)
	}
	if statuses[completedID].Status != aichatmodel.MessageStatusCompleted {
		t.Fatalf("completed status = %q, want completed", statuses[completedID].Status)
	}
	var cleaned aichatmodel.Conversation
	if err := db.Where("id = ?", conversationID).Take(&cleaned).Error; err != nil {
		t.Fatalf("load cleaned conversation: %v", err)
	}
	if cleaned.RuntimeStatus != aichatmodel.ConversationRuntimeStatusIdle {
		t.Fatalf("runtime_status = %q, want idle", cleaned.RuntimeStatus)
	}
	if cleaned.ActiveMessageID != nil {
		t.Fatalf("active_message_id = %v, want nil", cleaned.ActiveMessageID)
	}
}

func TestService_StreamConversationEventsReplaysRedisEvents(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	withRedisClient(t)

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"hello", " world"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}
	if _, err := svc.RunPreparedStream(context.Background(), prepared, nil); err != nil {
		t.Fatalf("RunPreparedStream() error = %v", err)
	}

	var events []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, prepared.Conversation.ID, prepared.Message.ID, "0", func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3: %#v", len(events), events)
	}
	if events[0].ID == "" || events[1].ID == "" || events[2].ID == "" {
		t.Fatalf("events should include redis stream ids: %#v", events)
	}
	if events[0].EventType != "message_start" || events[1].EventType != "message" || events[2].EventType != "message_end" {
		t.Fatalf("event types = [%s %s %s], want start/message/end", events[0].EventType, events[1].EventType, events[2].EventType)
	}
	if events[1].Payload["answer"] != "hello world" {
		t.Fatalf("message payload answer = %#v, want hello world", events[1].Payload["answer"])
	}

	var tail []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, prepared.Conversation.ID, prepared.Message.ID, events[0].ID, func(event aichatservice.StreamEvent) error {
		tail = append(tail, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() after first error = %v", err)
	}
	if len(tail) != 2 || tail[0].EventType != "message" || tail[1].EventType != "message_end" {
		t.Fatalf("tail events = %#v, want message and message_end", tail)
	}
}

func TestService_StreamConversationEventsReplaysArtifactWithMissingFileFields(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	withRedisClient(t)

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{chunks: []string{"unused"}})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	prepared, err := svc.PrepareChat(context.Background(), scope, aichatdto.ChatRequest{Query: "hello", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("PrepareChat() error = %v", err)
	}

	client := redisutil.GetClient()
	streamKey := "aichat:message:" + prepared.Message.ID.String() + ":events"
	artifactPayload := fmt.Sprintf(`{"conversation_id":%q,"message_id":%q,"artifact_type":"file","filename":"missing.md","url":"old-url"}`, prepared.Conversation.ID.String(), prepared.Message.ID.String())
	endPayload := fmt.Sprintf(`{"conversation_id":%q,"message_id":%q,"status":"completed"}`, prepared.Conversation.ID.String(), prepared.Message.ID.String())
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"event_type":      "skill_artifact_created",
			"payload":         artifactPayload,
			"created_at":      fmt.Sprintf("%d", time.Now().Unix()),
		},
	}).Err(); err != nil {
		t.Fatalf("append artifact event: %v", err)
	}
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"event_type":      "message_end",
			"payload":         endPayload,
			"created_at":      fmt.Sprintf("%d", time.Now().Unix()),
		},
	}).Err(); err != nil {
		t.Fatalf("append terminal event: %v", err)
	}

	var events []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, prepared.Conversation.ID, prepared.Message.ID, "0", func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() error = %v", err)
	}
	artifact := firstEventPayload(events, "skill_artifact_created")
	if artifact == nil {
		t.Fatalf("events = %#v, want artifact event", events)
	}
	if artifact["url"] != "old-url" || artifact["download_url"] != nil {
		t.Fatalf("artifact payload = %#v, want unchanged missing-file metadata", artifact)
	}
	if !containsString(eventTypes(events), "message_end") {
		t.Fatalf("events = %#v, want terminal event", events)
	}
}

func TestService_StreamConversationEventsEmitsExpiredErrorForActiveMessageWithoutRedisEvents(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	seedMember(t, db, orgID, accountID)
	redisutil.SetClient(nil)

	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       orgID,
		AccountID:            accountID,
		Title:                "Expired stream",
		Status:               aichatmodel.ConversationStatusNormal,
		RuntimeStatus:        aichatmodel.ConversationRuntimeStatusStreaming,
		ActiveMessageID:      &messageID,
		Source:               aichatmodel.ConversationSourceConsole,
		CurrentLeafMessageID: nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message := newStoredMessage(messageID, conversationID, nil, "hello", now)
	message.Status = aichatmodel.MessageStatusStreaming
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	scope := aichatservice.Scope{OrganizationID: orgID, AccountID: accountID}
	var events []aichatservice.StreamEvent
	if err := svc.StreamConversationEvents(context.Background(), scope, conversationID, messageID, "0", func(event aichatservice.StreamEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamConversationEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].EventType != "error" || events[0].Payload["message"] != "stream events expired" {
		t.Fatalf("event = %#v, want expired error", events[0])
	}
}

func TestRepository_ListBranchDoesNotDuplicateMessages(t *testing.T) {
	db := openAIChatTestDB(t)
	conversationID := uuid.New()
	rootID := uuid.New()
	leafID := uuid.New()
	now := time.Now()
	conversation := &aichatmodel.Conversation{
		ID:             conversationID,
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Title:          "Branch test",
		Status:         aichatmodel.ConversationStatusNormal,
		Source:         aichatmodel.ConversationSourceConsole,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(newStoredMessage(rootID, conversationID, nil, "root", now)).Error; err != nil {
		t.Fatalf("create root message: %v", err)
	}
	if err := db.Create(newStoredMessage(leafID, conversationID, &rootID, "leaf", now.Add(time.Second))).Error; err != nil {
		t.Fatalf("create leaf message: %v", err)
	}

	branch, err := repository.NewRepositories(db).Message.ListBranch(context.Background(), leafID, 10)
	if err != nil {
		t.Fatalf("ListBranch() error = %v", err)
	}
	if len(branch) != 2 {
		t.Fatalf("branch length = %d, want 2", len(branch))
	}
	if branch[0].ID != rootID || branch[1].ID != leafID {
		t.Fatalf("branch ids = [%s, %s], want [%s, %s]", branch[0].ID, branch[1].ID, rootID, leafID)
	}
}

func TestService_MigrateExistingConversationRejectsOtherAccount(t *testing.T) {
	db := openAIChatTestDB(t)
	orgID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	sourceConversationID := uuid.New()
	seedMember(t, db, orgID, accountID)

	sourceID := sourceConversationID
	existing := &aichatmodel.Conversation{
		ID:                   uuid.New(),
		OrganizationID:       orgID,
		AccountID:            otherAccountID,
		Title:                "Existing",
		Status:               aichatmodel.ConversationStatusNormal,
		Source:               aichatmodel.ConversationSourceWebApp,
		SourceConversationID: &sourceID,
	}
	if err := db.Create(existing).Error; err != nil {
		t.Fatalf("create existing conversation: %v", err)
	}

	svc := aichatservice.NewService(repository.NewRepositories(db), &fakeLLMClient{})
	_, err := svc.MigrateWebAppConversation(context.Background(), aichatservice.Scope{
		OrganizationID: orgID,
		AccountID:      accountID,
	}, sourceConversationID)
	if !errors.Is(err, aichatservice.ErrPermissionDenied) {
		t.Fatalf("MigrateWebAppConversation() error = %v, want ErrPermissionDenied", err)
	}
}

func newStoredMessage(id, conversationID uuid.UUID, parentID *uuid.UUID, query string, createdAt time.Time) *aichatmodel.Message {
	return &aichatmodel.Message{
		ID:              id,
		ConversationID:  conversationID,
		ParentID:        parentID,
		Query:           query,
		Answer:          "answer",
		Status:          aichatmodel.MessageStatusCompleted,
		ModelName:       "gpt-test",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{},
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
}

func formatTurnQuery(index int) string {
	return fmt.Sprintf("turn-%02d", index)
}

func formatTurnAnswer(index int) string {
	return fmt.Sprintf("answer-%02d", index)
}

func containsMessageContentPrefix(messages []adapter.Message, prefix string) bool {
	for _, message := range messages {
		content, ok := message.Content.(string)
		if ok && strings.HasPrefix(content, prefix) {
			return true
		}
	}
	return false
}

func containsExactAssistantContent(messages []adapter.Message, content string) bool {
	for _, message := range messages {
		if message.Role != "assistant" {
			continue
		}
		if text, ok := message.Content.(string); ok && text == content {
			return true
		}
	}
	return false
}

func containsUserContent(messages []adapter.Message, fragment string) bool {
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		if text, ok := message.Content.(string); ok && strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func eventTypes(events []aichatservice.StreamEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.EventType)
	}
	return types
}

func metadataFilesFromValue(value interface{}) []map[string]interface{} {
	switch files := value.(type) {
	case []map[string]interface{}:
		return files
	case []interface{}:
		output := make([]map[string]interface{}, 0, len(files))
		for _, item := range files {
			if file, ok := item.(map[string]interface{}); ok {
				output = append(output, file)
			}
		}
		return output
	default:
		return nil
	}
}

func metadataInt(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func metadataMap(t *testing.T, metadata map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	value, ok := metadata[key]
	if !ok {
		t.Fatalf("metadata[%q] is missing", key)
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	default:
		t.Fatalf("metadata[%q] = %#v, want map", key, value)
		return nil
	}
}

func metadataString(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func metadataStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func metadataInt64(value interface{}) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	default:
		return 0
	}
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func newTestUploadFile(id, organizationID, createdBy string, workspaceID *string, temporary bool) *sharedto.UploadFile {
	return &sharedto.UploadFile{
		ID:             id,
		TenantID:       organizationID,
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Name:           "document.txt",
		Size:           128,
		Extension:      "txt",
		MimeType:       "text/plain",
		CreatedBy:      createdBy,
		IsTemporary:    temporary,
	}
}

func openAIChatTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	createAIChatTestTables(t, db)
	if err := db.Exec(`CREATE TABLE members (organization_id TEXT NOT NULL, account_id TEXT NOT NULL)`).Error; err != nil {
		t.Fatalf("create members: %v", err)
	}
	if err := db.Exec(`CREATE TABLE account_contexts (account_id TEXT NOT NULL, current_workspace_id TEXT)`).Error; err != nil {
		t.Fatalf("create account_contexts: %v", err)
	}
	return db
}

func createAIChatTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqls := []string{
		`CREATE TABLE aichat_conversations (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			workspace_id TEXT,
			account_id TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			runtime_status TEXT NOT NULL DEFAULT 'idle',
			current_leaf_message_id TEXT,
			active_message_id TEXT,
			dialogue_count INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL,
			source_conversation_id TEXT,
			source_web_app_id TEXT,
			metadata TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE aichat_messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			parent_id TEXT,
			query TEXT NOT NULL,
			answer TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			error TEXT,
			model_provider TEXT,
			model_name TEXT NOT NULL,
			billing_reason_source TEXT,
			model_parameters TEXT NOT NULL DEFAULT '{}',
			metadata TEXT NOT NULL DEFAULT '{}',
			source_message_id TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE aichat_organization_skill_configs (
			organization_id TEXT NOT NULL,
			skill_id TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME,
			updated_at DATETIME,
			PRIMARY KEY (organization_id, skill_id)
		)`,
		`CREATE TABLE aichat_custom_skills (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			skill_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			when_to_use TEXT NOT NULL,
			runtime_type TEXT NOT NULL DEFAULT 'prompt',
			display TEXT NOT NULL DEFAULT '{}',
			storage_path TEXT NOT NULL,
			manifest TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'active',
			validation_error TEXT,
			created_by TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
	}
	for _, sql := range sqls {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("create aichat table: %v", err)
		}
	}
}

func seedMember(t *testing.T, db *gorm.DB, orgID, accountID uuid.UUID) {
	t.Helper()
	if err := db.Exec(`INSERT INTO members (organization_id, account_id) VALUES (?, ?)`, orgID.String(), accountID.String()).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

func seedAccountContext(t *testing.T, db *gorm.DB, accountID uuid.UUID, workspaceID *uuid.UUID) {
	t.Helper()
	var workspace any
	if workspaceID != nil {
		workspace = workspaceID.String()
	}
	if err := db.Exec(`INSERT INTO account_contexts (account_id, current_workspace_id) VALUES (?, ?)`, accountID.String(), workspace).Error; err != nil {
		t.Fatalf("seed account context: %v", err)
	}
}

func seedOrganizationSkillConfigs(t *testing.T, db *gorm.DB, orgID uuid.UUID, configs map[string]bool) {
	t.Helper()
	now := time.Now()
	for skillID, enabled := range configs {
		if err := db.Exec(
			`INSERT INTO aichat_organization_skill_configs (organization_id, skill_id, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			orgID.String(),
			skillID,
			enabled,
			now,
			now,
		).Error; err != nil {
			t.Fatalf("seed organization skill config: %v", err)
		}
	}
}

func withRedisClient(t *testing.T) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisutil.SetClient(nil)
	})
	redisutil.SetClient(client)
}

type fakeLLMClient struct {
	chunks           []string
	streamErr        error
	blockUntilCancel bool
	started          chan struct{}
	startOnce        sync.Once
	requests         []*adapter.ChatRequest
	appChatRequests  []*adapter.ChatRequest
	appChatResponses []*adapter.ChatResponse
	appChatErr       error
	appContexts      []*llmclient.AppContext
}

type fakeModelSpecResolver struct {
	spec aichatservice.ModelSpec
	ok   bool
	err  error
}

func functionCallingModelResolver() *fakeModelSpecResolver {
	return &fakeModelSpecResolver{ok: true, spec: aichatservice.ModelSpec{ContextWindow: 4096, SupportsToolCall: true}}
}

func (f *fakeModelSpecResolver) Resolve(context.Context, uuid.UUID, string, string) (aichatservice.ModelSpec, bool, error) {
	if f.err != nil {
		return aichatservice.ModelSpec{}, false, f.err
	}
	if !f.ok && f.spec.ContextWindow == 0 {
		return aichatservice.ModelSpec{}, false, nil
	}
	return f.spec, true, nil
}

type fakeAIChatFileService struct {
	limit int
	files map[string]*sharedto.UploadFile
	urls  map[string]string
}

func (f *fakeAIChatFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{WorkflowFileUploadLimit: f.limit}
}

func (f *fakeAIChatFileService) GetFileByID(ctx context.Context, fileID string) (*sharedto.UploadFile, error) {
	file, ok := f.files[fileID]
	if !ok {
		return nil, errors.New("file not found")
	}
	return file, nil
}

func (f *fakeAIChatFileService) GetFileURL(ctx context.Context, fileID string) (string, error) {
	if f.urls != nil {
		if url, ok := f.urls[fileID]; ok {
			return url, nil
		}
	}
	if _, ok := f.files[fileID]; !ok {
		return "", errors.New("file not found")
	}
	return "https://files.example.com/" + fileID, nil
}

type fakeAIChatContentExtractor struct {
	contents  map[string]string
	errors    map[string]error
	fromCache map[string]bool
}

func (f *fakeAIChatContentExtractor) ExtractMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*workflowfile.FileContent, error) {
	results := make([]*workflowfile.FileContent, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		content := ""
		if f.contents != nil {
			content = f.contents[fileID]
		}
		var err error
		if f.errors != nil {
			err = f.errors[fileID]
		}
		results = append(results, &workflowfile.FileContent{
			FileID:      fileID,
			Content:     content,
			ContentType: "text/plain",
			Size:        len(content),
			Error:       err,
			FromCache:   f.fromCache != nil && f.fromCache[fileID],
		})
	}
	return results, nil
}

type fakeWorkspacePermissionService struct {
	allowed          bool
	lastWorkspaceID  string
	lastPermission   workspacemodel.WorkspacePermissionCode
	lastAccountID    string
	lastOrganization string
}

func (f *fakeWorkspacePermissionService) CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error) {
	f.lastOrganization = organizationID
	f.lastWorkspaceID = workspaceID
	f.lastAccountID = accountID
	f.lastPermission = permissionCode
	return f.allowed, nil
}

type fakeTitleGenerator struct {
	mu          sync.Mutex
	title       string
	calls       int
	lastRequest titlegen.GenerateRequest
}

func (f *fakeTitleGenerator) Generate(ctx context.Context, req titlegen.GenerateRequest) (*titlegen.GenerateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastRequest = req
	title := f.title
	if title == "" {
		title = req.FallbackTitle
	}
	return &titlegen.GenerateResult{Title: title, Source: titlegen.SourceModel}, nil
}

func (f *fakeTitleGenerator) snapshot() (int, titlegen.GenerateRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.lastRequest
}

func (f *fakeLLMClient) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	f.requests = append(f.requests, req)
	ch := make(chan adapter.StreamResponse, len(f.chunks)+2)
	go func() {
		defer close(ch)
		if f.blockUntilCancel {
			if f.started != nil {
				f.startOnce.Do(func() { close(f.started) })
			}
			<-ctx.Done()
			return
		}
		if f.streamErr != nil {
			ch <- adapter.StreamResponse{Error: f.streamErr}
			return
		}
		for _, chunk := range f.chunks {
			ch <- adapter.StreamResponse{
				Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: chunk}}},
			}
		}
		ch <- adapter.StreamResponse{
			Done: true,
			Usage: &adapter.Usage{
				PromptTokens:     1,
				CompletionTokens: 2,
				TotalTokens:      3,
			},
		}
	}()
	return ch, nil
}

func (f *fakeLLMClient) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.appContexts = append(f.appContexts, appCtx)
	f.appChatRequests = append(f.appChatRequests, req)
	if f.appChatErr != nil {
		return nil, f.appChatErr
	}
	if len(f.appChatResponses) == 0 {
		return nil, errors.New("not implemented")
	}
	resp := f.appChatResponses[0]
	f.appChatResponses = f.appChatResponses[1:]
	return resp, nil
}

func (f *fakeLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	f.appContexts = append(f.appContexts, appCtx)
	return f.ChatStream(ctx, appCtx.OrganizationID, req)
}

func (f *fakeLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}
