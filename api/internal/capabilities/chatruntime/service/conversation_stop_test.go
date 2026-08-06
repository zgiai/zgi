package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStopConversationStopsWaitingLeafMessage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()

	db := openStopConversationTestDB(t)
	conversation := &runtimemodel.Conversation{
		ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
		CallerType: runtimemodel.ConversationCallerAIChat, ConversationType: runtimemodel.ConversationTypeChat,
		Title: "stop", Status: runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusStreaming,
		CurrentLeafMessageID: &messageID, ActiveMessageID: &messageID,
	}
	message := &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Query: "run", Answer: "七零八落\n一心一意\n",
		Status: runtimemodel.MessageStatusWaitingApproval, ModelName: "test",
		Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{
				"workflow_run_id": "workflow-run-1",
				"status":          workflowContinuationStatusWaitingApproval,
			},
		},
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	repos := repository.NewRepositories(db)
	repos.Access = stopConversationAccessRepo{}
	svc := &service{
		repos:   repos,
		streams: newStreamRegistry(),
	}

	result, err := svc.StopConversation(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID)
	if err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	if result.Message == nil {
		t.Fatal("StopConversation() message = nil, want stopped waiting message")
	}
	if result.Message.Status != runtimemodel.MessageStatusStopped {
		t.Fatalf("message status = %q, want %q", result.Message.Status, runtimemodel.MessageStatusStopped)
	}
	if result.Message.Answer != "七零八落\n一心一意\n" {
		t.Fatalf("stopped answer = %q, want complete persisted answer", result.Message.Answer)
	}
	var persistedConversation runtimemodel.Conversation
	if err := db.First(&persistedConversation, "id = ?", conversationID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedConversation.ActiveMessageID != nil {
		t.Fatalf("active message = %v, want nil", persistedConversation.ActiveMessageID)
	}
}

func TestStopConversationIgnoresCompletedLeafMessage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()

	db := openStopConversationTestDB(t)
	conversation := &runtimemodel.Conversation{
		ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
		CallerType: runtimemodel.ConversationCallerAIChat, ConversationType: runtimemodel.ConversationTypeChat,
		Title: "completed", Status: runtimemodel.ConversationStatusNormal,
		RuntimeStatus: runtimemodel.ConversationRuntimeStatusIdle, CurrentLeafMessageID: &messageID,
	}
	message := &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Query: "done", Status: runtimemodel.MessageStatusCompleted, ModelName: "test",
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	repos := repository.NewRepositories(db)
	repos.Access = stopConversationAccessRepo{}
	svc := &service{
		repos:   repos,
		streams: newStreamRegistry(),
	}

	result, err := svc.StopConversation(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID)
	if err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	if result.Message != nil {
		t.Fatalf("StopConversation() message = %#v, want nil", result.Message)
	}
}

func TestStopMessageDoesNotOverwriteNewerAnswerFromStaleSnapshot(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	db := openStopConversationTestDB(t)
	conversation := &runtimemodel.Conversation{
		ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
		CallerType: runtimemodel.ConversationCallerAgent, ConversationType: runtimemodel.ConversationTypeChat,
		Title: "stale", Status: runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusStreaming,
		CurrentLeafMessageID: &messageID, ActiveMessageID: &messageID,
	}
	message := &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Query: "run", Answer: "七零八落\n一心",
		Status: runtimemodel.MessageStatusStreaming, ModelName: "test",
		Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run-stale"},
		},
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate the pre-fix race: one observer still holds this old snapshot
	// while the continuation producer commits the complete visible answer.
	var stale runtimemodel.Message
	if err := db.First(&stale, "id = ?", messageID).Error; err != nil {
		t.Fatal(err)
	}
	fullAnswer := "七零八落\n一心一意\n"
	if err := db.Model(&runtimemodel.Message{}).Where("id = ?", messageID).Update("answer", fullAnswer).Error; err != nil {
		t.Fatal(err)
	}

	repos := repository.NewRepositories(db)
	repos.Access = stopConversationAccessRepo{}
	svc := &service{repos: repos, streams: newStreamRegistry()}
	stopped, err := svc.StopMessage(t.Context(), Scope{OrganizationID: organizationID, AccountID: accountID}, messageID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Answer != fullAnswer {
		t.Fatalf("stale snapshot %q overwrote final answer %q; persisted %q", stale.Answer, fullAnswer, stopped.Answer)
	}
}

func openStopConversationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.Conversation{}, &runtimemodel.Message{}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatal(err)
	}
	return db
}

type stopConversationAccessRepo struct {
	repository.AccessRepository
}

func (stopConversationAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type stopConversationRepo struct {
	repository.ConversationRepository
	conversation      *runtimemodel.Conversation
	clearActiveCalled bool
}

func (r *stopConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *stopConversationRepo) ClearActiveMessage(context.Context, uuid.UUID, uuid.UUID) error {
	r.clearActiveCalled = true
	return nil
}

type stopConversationMessageRepo struct {
	repository.MessageRepository
	message             *runtimemodel.Message
	updateStoppedCalled bool
}

func (r *stopConversationMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}

func (r *stopConversationMessageRepo) UpdateStoppedAnswer(_ context.Context, _ uuid.UUID, answer string, metadata map[string]interface{}) error {
	r.updateStoppedCalled = true
	r.message.Answer = answer
	r.message.Metadata = metadata
	r.message.Status = runtimemodel.MessageStatusStopped
	return nil
}
