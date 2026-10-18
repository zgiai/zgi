package agents

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestRuntimeInputUserStatus(t *testing.T) {
	followUp := runtimemodel.RuntimeInputConsumptionFollowUp
	current := runtimemodel.RuntimeInputConsumptionCurrentRun
	for name, test := range map[string]struct {
		input *runtimemodel.RuntimeInput
		want  string
	}{
		"pending":          {input: &runtimemodel.RuntimeInput{Status: runtimemodel.RuntimeInputStatusPending}, want: "pending"},
		"claimed current":  {input: &runtimemodel.RuntimeInput{Status: runtimemodel.RuntimeInputStatusClaimed, ConsumptionMode: &current}, want: "pending"},
		"consumed current": {input: &runtimemodel.RuntimeInput{Status: runtimemodel.RuntimeInputStatusConsumed, ConsumptionMode: &current}, want: "applied_current_run"},
		"claimed followup": {input: &runtimemodel.RuntimeInput{Status: runtimemodel.RuntimeInputStatusClaimed, ConsumptionMode: &followUp}, want: "queued_follow_up"},
		"consumed followup": {input: &runtimemodel.RuntimeInput{
			Status: runtimemodel.RuntimeInputStatusConsumed, ConsumptionMode: &followUp,
		}, want: "applied_follow_up"},
		"canceled": {input: &runtimemodel.RuntimeInput{Status: runtimemodel.RuntimeInputStatusCanceled}, want: "canceled"},
		"failed":   {input: &runtimemodel.RuntimeInput{Status: runtimemodel.RuntimeInputStatusFailed}, want: "failed"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := runtimeInputUserStatus(test.input); got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRuntimeInputsMoveToConsumedFollowUpMessage(t *testing.T) {
	targetMessageID := uuid.New()
	followUpMessageID := uuid.New()
	followUp := runtimemodel.RuntimeInputConsumptionFollowUp
	inputs := []*runtimemodel.RuntimeInput{
		{
			ID: uuid.New(), ClientInputID: uuid.New(), TargetMessageID: targetMessageID,
			Status: runtimemodel.RuntimeInputStatusPending, Content: "waiting supplement",
		},
		{
			ID: uuid.New(), ClientInputID: uuid.New(), TargetMessageID: targetMessageID,
			ConsumedMessageID: &followUpMessageID, ConsumptionMode: &followUp,
			Status: runtimemodel.RuntimeInputStatusConsumed, Content: "follow-up supplement",
		},
	}
	projection := runtimeInputsByVisibleMessage(inputs)
	if len(projection[targetMessageID]) != 1 || len(projection[followUpMessageID]) != 0 {
		t.Fatalf("projection = %#v", projection)
	}
	if projection[targetMessageID][0].Content != "waiting supplement" {
		t.Fatalf("projection = %#v, want only the not-yet-migrated supplement", projection)
	}
}

func TestRuntimeMessageResponseShowsFollowUpQuery(t *testing.T) {
	message := &runtimemodel.Message{
		ID: uuid.New(), ConversationID: uuid.New(), Query: "next user message",
		Metadata: map[string]interface{}{"runtime_input_internal_query": true},
	}
	response := runtimeMessageResponse(message, runtimeservice.Caller{})
	if response.Query != message.Query {
		t.Fatalf("query = %q, want %q", response.Query, message.Query)
	}
}
