package digitalhuman

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestMessageHistoryReturnsApprovalTokensOnlyForAnApprovalTheReaderMayAnswer(t *testing.T) {
	message := func(status string, allowed bool) *runtimemodel.Message {
		return &runtimemodel.Message{Status: status, Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{"ui_approval_allowed": allowed, "approval_token": "secret-token"},
			"workflow_runs": []interface{}{map[string]interface{}{
				"approval": map[string]interface{}{"approval_token": "secret-token", "approval_form": map[string]interface{}{"id": "form-1", "token": "secret-token"}},
				"nodes": []interface{}{map[string]interface{}{"outputs": map[string]interface{}{
					"__approval_token": "secret-token", "__approval_form": map[string]interface{}{"id": "form-1", "token": "secret-token"}, "text": "kept",
				}}},
			}},
		}}
	}
	for name, test := range map[string]struct {
		message *runtimemodel.Message
		source  string
		want    bool
	}{
		"approval answered in the webapp":         {message(runtimemodel.MessageStatusWaitingApproval, true), runtimemodel.ConversationSourceWebApp, true},
		"approval answered through the API":       {message(runtimemodel.MessageStatusWaitingApproval, true), runtimemodel.ConversationSourceExternalAPI, true},
		"approval kept with its approvers":        {message(runtimemodel.MessageStatusWaitingApproval, false), runtimemodel.ConversationSourceExternalAPI, false},
		"question asked before an email approval": {message(runtimemodel.MessageStatusWaitingQuestion, false), runtimemodel.ConversationSourceWebApp, false},
		"settled message":                         {message(runtimemodel.MessageStatusCompleted, true), runtimemodel.ConversationSourceWebApp, false},
		"console owner on a waiting approval":     {message(runtimemodel.MessageStatusWaitingApproval, false), runtimemodel.ConversationSourceConsole, true},
		"channel reader":                          {message(runtimemodel.MessageStatusWaitingApproval, true), runtimemodel.ConversationSourceChannel, false},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(digitalHumanMessageResponse(test.message, test.source).Metadata)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(encoded), "secret-token"); got != test.want {
				t.Fatalf("metadata = %s, want the approval token returned %v", encoded, test.want)
			}
			if !strings.Contains(string(encoded), "kept") || !strings.Contains(string(encoded), "form-1") {
				t.Fatalf("metadata = %s, want everything but the credentials kept", encoded)
			}
		})
	}
}
