package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"gorm.io/gorm"
)

// TaskApprovalNotice tells an API end user that a change to one of their
// tasks waits for their approval in a conversation. Integrators learn of it
// through the end user's notifications and the DigitalHuman's webhook; the
// approval itself is answered through the conversation's workflow
// continuation.
type TaskApprovalNotice struct {
	OrganizationID  uuid.UUID
	WorkspaceID     uuid.UUID
	DigitalHumanID  uuid.UUID
	RecipientKind   string
	RecipientID     uuid.UUID
	TaskID          uuid.UUID
	TaskName        string
	TaskAction      string
	ClientReference string
	ConversationID  uuid.UUID
	MessageID       uuid.UUID
}

// TaskApprovalNotifier is implemented by the inbox and injected into the
// runtime after both are built.
type TaskApprovalNotifier interface {
	NotifyTaskApproval(context.Context, TaskApprovalNotice) error
}

// SetTaskApprovalNotifier lets the runtime tell end users about approvals
// that wait for them.
func (s *RuntimeService) SetTaskApprovalNotifier(notifier TaskApprovalNotifier) {
	if s != nil {
		s.approvalNotifier = notifier
	}
}

// pendingTaskApprovalReader finds the task approval a waiting run paused on.
type pendingTaskApprovalReader interface {
	PendingTaskApproval(context.Context, *Run) (*TaskApprovalNotice, error)
}

// notifyTaskApproval tells an API end user that their run paused on a task
// approval. People in the console and WebApp answer approvals where they
// chat; API end users may be gone by then, so their integrator is told. A
// failure is logged by the caller's absence of effect only: the approval
// still waits in the conversation.
func (s *RuntimeService) notifyTaskApproval(ctx context.Context, run *Run) {
	if s.approvalNotifier == nil || run == nil || run.Surface != SurfaceAPI || run.UserSubjectID == nil ||
		run.UserSubjectKind != string(usersubject.APIEndUser) {
		return
	}
	reader, ok := s.store.(pendingTaskApprovalReader)
	if !ok {
		return
	}
	notice, err := reader.PendingTaskApproval(ctx, run)
	if err != nil || notice == nil {
		return
	}
	notice.RecipientKind, notice.RecipientID = run.UserSubjectKind, *run.UserSubjectID
	_ = s.approvalNotifier.NotifyTaskApproval(context.WithoutCancel(ctx), *notice)
}

// PendingTaskApproval returns the task approval message a waiting run paused
// on, with the task's client reference, or nil when the run waits on
// something else.
func (r *GormRepository) PendingTaskApproval(ctx context.Context, run *Run) (*TaskApprovalNotice, error) {
	if r == nil || r.db == nil || run == nil || run.ConversationID == nil || run.MessageID == nil {
		return nil, nil
	}
	var row struct {
		MessageID       uuid.UUID
		TaskID          string
		TaskName        string
		TaskAction      string
		ClientReference *string
	}
	err := r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Select(`m.id AS message_id,
			m.metadata->'agent_workflow_continuation'->>'task_id' AS task_id,
			m.metadata->'agent_workflow_continuation'->>'task_name' AS task_name,
			m.metadata->'agent_workflow_continuation'->>'task_action' AS task_action,
			t.client_reference`).
		Joins("LEFT JOIN automation_tasks AS t ON t.id::text = m.metadata->'agent_workflow_continuation'->>'task_id' AND t.subject_id = ?", run.DigitalHumanID).
		Where("m.id = ? AND m.conversation_id = ? AND m.deleted_at IS NULL AND m.status = ?", *run.MessageID, *run.ConversationID, runtimemodel.MessageStatusWaitingApproval).
		Where("m.metadata->'agent_workflow_continuation'->>'kind' = ?", "digital_human_task").
		Order("m.created_at DESC").Limit(1).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load pending DigitalHuman task approval: %w", err)
	}
	notice := &TaskApprovalNotice{
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, DigitalHumanID: run.DigitalHumanID,
		TaskName: strings.TrimSpace(row.TaskName), TaskAction: strings.TrimSpace(row.TaskAction),
		ConversationID: *run.ConversationID, MessageID: row.MessageID,
	}
	if row.ClientReference != nil {
		notice.ClientReference = *row.ClientReference
	}
	// A proposal to create a task has no task yet.
	if id, err := uuid.Parse(strings.TrimSpace(row.TaskID)); err == nil {
		notice.TaskID = id
	}
	return notice, nil
}
