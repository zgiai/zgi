package digitalhumantask

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestTaskReportOriginNamesWhereTheTaskWasAsked(t *testing.T) {
	console := digitalhuman.SurfaceConsole
	channel := digitalhuman.SurfaceChannel
	conversation := "6f1f7f9e-4b0e-4c1e-9a8e-2d1f6f0c9a11"
	consoleTask := &automationmodel.AutomationTask{OriginSurface: &console, SourceSnapshot: map[string]interface{}{"conversation_id": conversation}}

	if origin := taskReportOrigin(consoleTask, true); origin == nil || origin.Kind != "conversation" || origin.ConversationID != conversation {
		t.Fatalf("holder's console origin = %+v", origin)
	}
	// Someone else's conversation is never named.
	if origin := taskReportOrigin(consoleTask, false); origin != nil {
		t.Fatalf("non-holder console origin = %+v", origin)
	}
	channelTask := &automationmodel.AutomationTask{OriginSurface: &channel,
		SourceSnapshot: map[string]interface{}{"conversation_id": "oc_external", "conversation_type": "group"},
		DeliveryConfig: map[string]any{"reply_to": map[string]any{"provider": "feishu"}}}
	origin := taskReportOrigin(channelTask, false)
	if origin == nil || origin.Kind != "channel" || origin.ConversationType != "group" || origin.Provider != "feishu" || origin.ConversationID != "" {
		t.Fatalf("channel origin = %+v", origin)
	}
	if origin := taskReportOrigin(&automationmodel.AutomationTask{OriginSurface: &console}, true); origin != nil {
		t.Fatalf("task without a conversation has origin %+v", origin)
	}
}

// A task created in the task center is held by the member who created it, who
// reads its reports as their own; another member does not hold it.
func TestTaskCenterTaskIsHeldByItsCreator(t *testing.T) {
	owner := automationmodel.AutomationTaskOwnerDigitalHuman
	creator := "81031798-75f3-49aa-8ca1-f3ad9012d653"
	task := &automationmodel.AutomationTask{OwnerKind: &owner, CreatedBy: creator}
	reader := func(id string) digitalhuman.TaskAccessContext {
		return digitalhuman.TaskAccessContext{Principal: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: id},
			Owner: digitalhuman.UserTaskOwner(usersubject.Ref{Kind: usersubject.Account, ID: uuid.MustParse(id)})}
	}
	if !taskHeldBy(task, reader(creator)) {
		t.Fatal("the creator does not hold their task center task")
	}
	if taskHeldBy(task, reader(uuid.NewString())) {
		t.Fatal("another member holds the task")
	}
}
