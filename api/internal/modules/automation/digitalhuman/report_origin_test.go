package digitalhumantask

import (
	"testing"

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
