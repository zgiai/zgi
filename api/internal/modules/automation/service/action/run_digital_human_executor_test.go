package action

import (
	"context"
	"testing"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

type capturingDigitalHumanRunner struct{ request DigitalHumanRunRequest }

func (r *capturingDigitalHumanRunner) RunDigitalHuman(_ context.Context, request DigitalHumanRunRequest) (*DigitalHumanRunResult, error) {
	r.request = request
	return &DigitalHumanRunResult{RunID: uuid.NewString()}, nil
}
func (*capturingDigitalHumanRunner) GetDigitalHumanRun(context.Context, string, string, string, string) (*DigitalHumanRunState, error) {
	return nil, nil
}
func (*capturingDigitalHumanRunner) CancelDigitalHumanRun(context.Context, string, string, string, string) error {
	return nil
}

func TestRunDigitalHumanExecutorReturnsPendingExternalRun(t *testing.T) {
	runner := &capturingDigitalHumanRunner{}
	executor := NewRunDigitalHumanExecutor(func() AutomationDigitalHumanRunner { return runner })
	subjectID := uuid.NewString()
	ownerKind := automationmodel.AutomationTaskOwnerConversation
	ownerConversationID, originSurface := uuid.NewString(), "channel"
	goal := "produce the agreed result"
	task := &automationmodel.AutomationTask{
		ID: uuid.NewString(), OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), SubjectID: &subjectID,
		OwnerKind: &ownerKind, OwnerConversationID: &ownerConversationID, OriginSurface: &originSurface,
		Name: "Scheduled work", Goal: &goal, SuccessCriteria: []string{"done"}, ExecutionTimeoutSeconds: 300,
		CreatedBy: uuid.NewString(), SourceRef: stringPointer("channel:feishu"), SourceSnapshot: map[string]any{"installation_id": "installation-1", "conversation_id": "chat-1"}, DeliveryConfig: map[string]any{"mode": "source_conversation", "reply_to": map[string]any{"provider": "feishu", "installation_id": "installation-1", "conversation_id": "chat-1", "recipient_id": "ou_creator"}},
	}
	taskRun := &automationmodel.AutomationTaskRun{ID: uuid.NewString(), TaskID: task.ID}
	actionRun := &automationmodel.AutomationActionRun{ID: uuid.NewString(), TaskRunID: taskRun.ID}
	result, err := executor.ExecuteAction(t.Context(), ActionExecutionRequest{Task: task, TaskRun: taskRun, ActionRun: actionRun})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if result == nil || !result.Pending || result.ExternalRunType != "digital_human" || result.ExternalRunID == "" {
		t.Fatalf("ExecuteAction() result = %#v", result)
	}
	if runner.request.ActionRunID != actionRun.ID || runner.request.DigitalHumanID != subjectID || runner.request.CreatorAccountID != task.CreatedBy || runner.request.ReplyTo == nil ||
		runner.request.ReplyTo.ConversationID != "chat-1" || runner.request.ReplyTo.RecipientID != "ou_creator" ||
		runner.request.DeliveryMode != "source_conversation" ||
		runner.request.OwnerKind != string(ownerKind) || runner.request.OwnerConversationID != ownerConversationID ||
		runner.request.ChannelProvider != "feishu" || runner.request.ChannelInstallationID != "installation-1" || runner.request.ChannelConversationID != "chat-1" {
		t.Fatalf("runner request = %#v", runner.request)
	}
}

func stringPointer(value string) *string { return &value }
