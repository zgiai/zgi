package digitalhuman

import (
	"os"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPendingTaskChangesPostgresReturnsOnlyTheReadersOwnApprovals(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman pending task change PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID := uuid.New(), uuid.New()
	reader, other := uuid.New(), uuid.New()
	digitalHumanID, otherDigitalHuman := uuid.New(), uuid.New()
	taskID, otherTask := uuid.New(), uuid.New()
	conversationFrom := func(account, caller uuid.UUID, source string) uuid.UUID {
		row := &runtimemodel.Conversation{ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: &workspaceID, AccountID: account,
			Title: "pending", CallerType: "digital_human", CallerID: &caller, Source: source, Metadata: map[string]interface{}{}}
		if err := tx.Create(row).Error; err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	conversation := func(account, caller uuid.UUID) uuid.UUID {
		return conversationFrom(account, caller, digitalHumanConversationSource)
	}
	message := func(conversationID uuid.UUID, status string, task uuid.UUID) uuid.UUID {
		row := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Query: "change it", Status: status, ModelName: "test",
			ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{
				"agent_workflow_continuation": map[string]interface{}{"kind": "digital_human_task", "task_id": task.String(), "status": "waiting_approval"},
			}}
		if err := tx.Create(row).Error; err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	mine := conversation(reader, digitalHumanID)
	waiting := message(mine, runtimemodel.MessageStatusWaitingApproval, taskID)
	message(mine, runtimemodel.MessageStatusCompleted, otherTask)
	message(conversation(other, digitalHumanID), runtimemodel.MessageStatusWaitingApproval, otherTask)
	message(conversation(reader, otherDigitalHuman), runtimemodel.MessageStatusWaitingApproval, otherTask)
	// The reader's own WebApp conversation cannot be opened in the console chat.
	message(conversationFrom(reader, digitalHumanID, digitalHumanWebAppConversationSource), runtimemodel.MessageStatusWaitingApproval, otherTask)

	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: reader}
	got, err := NewRepository(tx).PendingTaskChanges(t.Context(), scope, digitalHumanID, []uuid.UUID{taskID, otherTask})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[taskID].ConversationID != mine || got[taskID].MessageID != waiting {
		t.Fatalf("pending changes = %#v, want only the reader's waiting approval for %s", got, taskID)
	}
}
