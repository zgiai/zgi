package digitalhuman

import (
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"testing"
)

func TestSocialExecutionArtifactRequiresLiveRunAndExactMessage(t *testing.T) {
	s, run, event := backgroundAuthorizationFixture(t, SurfaceTask)
	subject := execution.BackgroundSubject(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, *run.ConversationID, backgroundTaskEntry)
	scope, err := execution.NewScope(execution.VerifiedSubject{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ApplicationType: "digital_human", ApplicationID: run.DigitalHumanID, Entry: backgroundTaskEntry, User: subject, ConversationID: *run.ConversationID, PublicationRef: run.DigitalHumanVersionID.String()})
	if err != nil {
		t.Fatal(err)
	}
	conversation, message := run.ConversationID.String(), run.MessageID.String()
	file := &toolfile.ToolFile{ID: uuid.NewString(), TenantID: run.OrganizationID.String(), UserID: subject.ID.String(), ConversationID: &conversation, ExecutionIdentity: scope.Identity, ExecutionMessageID: &message}
	if _, err := s.AuthorizeBackgroundExecutionArtifact(t.Context(), run, event, file); err != nil {
		t.Fatal(err)
	}
	wrong := *file
	other := uuid.NewString()
	wrong.ExecutionMessageID = &other
	if _, err := s.AuthorizeBackgroundExecutionArtifact(t.Context(), run, event, &wrong); err == nil {
		t.Fatal("other message accepted")
	}
	if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", event.Context["task_id"]).Update("status", automationmodel.AutomationTaskStatusPaused).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthorizeBackgroundExecutionArtifact(t.Context(), run, event, file); err == nil {
		t.Fatal("paused task imported protected artifact")
	}
}
