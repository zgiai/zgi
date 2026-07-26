package agents

import (
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestPublicAgentRuntimeMessageMetadataExposesApprovalTokenOnlyForInteractiveSurface(t *testing.T) {
	metadata := map[string]interface{}{
		"agent_workflow_continuation": map[string]interface{}{
			"approval_token":      "secret",
			"ui_approval_allowed": true,
			"approval_form": map[string]interface{}{
				"id": "form-1", "token": "secret",
			},
		},
	}
	message := &runtimemodel.Message{Status: runtimemodel.MessageStatusWaitingApproval, Metadata: metadata}

	for _, source := range []string{
		runtimemodel.ConversationSourceConsole,
		runtimemodel.ConversationSourceWebApp,
		runtimemodel.ConversationSourceExternalAPI,
	} {
		got := publicAgentRuntimeMessageMetadata(message, runtimeservice.Caller{Source: source})
		continuation := got["agent_workflow_continuation"].(map[string]interface{})
		if continuation["approval_token"] != "secret" {
			t.Fatalf("source %q approval token = %#v, want secret", source, continuation["approval_token"])
		}
	}

	message.Metadata["agent_workflow_continuation"].(map[string]interface{})["ui_approval_allowed"] = false
	for _, source := range []string{
		runtimemodel.ConversationSourceWebApp,
		runtimemodel.ConversationSourceExternalAPI,
	} {
		got := publicAgentRuntimeMessageMetadata(message, runtimeservice.Caller{Source: source})
		continuation := got["agent_workflow_continuation"].(map[string]interface{})
		if _, exists := continuation["approval_token"]; exists {
			t.Fatalf("source %q exposed approval token: %#v", source, continuation)
		}
		form := continuation["approval_form"].(map[string]interface{})
		if _, exists := form["token"]; exists {
			t.Fatalf("source %q exposed nested approval token: %#v", source, form)
		}
	}
}

func TestPublicAgentRuntimeMessageMetadataRedactsClosedApprovalToken(t *testing.T) {
	message := &runtimemodel.Message{
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{
				"approval_token": "secret",
			},
		},
	}
	got := publicAgentRuntimeMessageMetadata(message, runtimeservice.Caller{Source: runtimemodel.ConversationSourceConsole})
	continuation := got["agent_workflow_continuation"].(map[string]interface{})
	if _, exists := continuation["approval_token"]; exists {
		t.Fatalf("completed message exposed approval token: %#v", continuation)
	}
}
