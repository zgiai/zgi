package digitalhuman

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// TaskPendingChange points to a proposed change to a task that waits for the
// reader's approval in one of the reader's own conversations.
type TaskPendingChange struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
}

// TaskPendingChangeSource finds task approvals that wait in the reader's own
// console conversations with one DigitalHuman.
type TaskPendingChangeSource interface {
	PendingTaskChanges(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]TaskPendingChange, error)
}

// PendingTaskChanges returns, per task, the latest approval that waits in a
// conversation the scope's account owns. Approvals in other people's
// conversations are never returned: only the conversation owner can act on
// them, and they may contain that person's request.
func (r *GormRepository) PendingTaskChanges(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]TaskPendingChange, error) {
	if r == nil || r.db == nil || scope.ActorID == uuid.Nil || digitalHumanID == uuid.Nil || len(taskIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(taskIDs))
	for _, id := range taskIDs {
		ids = append(ids, id.String())
	}
	var rows []struct {
		MessageID      uuid.UUID
		ConversationID uuid.UUID
		TaskID         string
	}
	err := r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Select("m.id AS message_id, m.conversation_id, m.metadata->'agent_workflow_continuation'->>'task_id' AS task_id").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		// Only console conversations open in the console chat; the same account
		// may also wait on approvals in WebApp or API conversations.
		Where("c.organization_id = ? AND c.workspace_id = ? AND c.account_id = ? AND c.caller_type = ? AND c.caller_id = ? AND c.source = ? AND c.deleted_at IS NULL",
			scope.OrganizationID, scope.WorkspaceID, scope.ActorID, "digital_human", digitalHumanID, digitalHumanConversationSource).
		Where("m.deleted_at IS NULL AND m.status = ?", "waiting_approval").
		Where("m.metadata->'agent_workflow_continuation'->>'kind' = ?", "digital_human_task").
		Where("m.metadata->'agent_workflow_continuation'->>'task_id' IN ?", ids).
		Order("m.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list pending DigitalHuman task approvals: %w", err)
	}
	result := make(map[uuid.UUID]TaskPendingChange, len(rows))
	for _, row := range rows {
		taskID, err := uuid.Parse(strings.TrimSpace(row.TaskID))
		if err != nil {
			continue
		}
		if _, seen := result[taskID]; !seen {
			result[taskID] = TaskPendingChange{ConversationID: row.ConversationID, MessageID: row.MessageID}
		}
	}
	return result, nil
}

// withPendingChanges marks tasks that wait for the reader's approval. A
// lookup failure leaves the tasks unmarked rather than failing the read.
func withPendingChanges(ctx context.Context, source TaskPendingChangeSource, scope Scope, digitalHumanID uuid.UUID, tasks []*Task) {
	if source == nil || len(tasks) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			ids = append(ids, task.ID)
		}
	}
	pending, err := source.PendingTaskChanges(ctx, scope, digitalHumanID, ids)
	if err != nil {
		return
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if change, ok := pending[task.ID]; ok {
			change := change
			task.PendingChange = &change
		}
	}
}
