package digitalhuman

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestEveryRunKnowsTheTimeInTheUsersTimezone(t *testing.T) {
	now := time.Date(2026, 10, 24, 7, 40, 0, 0, time.UTC)
	guidance := currentTimeGuidance(now, loadRunLocation("Asia/Shanghai"))
	for _, want := range []string{"2026-10-24 15:40", "Saturday", "Asia/Shanghai", "UTC+08:00", "2026-10-24T07:40:00Z", "give task schedules timezone Asia/Shanghai", "no timezone unless it differs"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("guidance %q lacks %q", guidance, want)
		}
	}
	if loadRunLocation("").String() != defaultRunTimezone || loadRunLocation("Not/AZone").String() != defaultRunTimezone ||
		loadRunLocation("Europe/Berlin").String() != "Europe/Berlin" {
		t.Fatal("an unset or unknown account timezone does not fall back to the platform default")
	}
}

func TestATaskScheduleWithoutTimezoneIsInTheUsers(t *testing.T) {
	created := []CreateTaskRequest{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{createCalls: &created})
	run, event := taskRuntimeFixture(invocation.OriginManual)
	prepared, err := provider.Prepare(withRunTimezone(t.Context(), loadRunLocation("Asia/Tokyo")), run, event)
	if err != nil {
		t.Fatal(err)
	}
	result := prepared.Tools[2].Handler(t.Context(), adapter.ToolCall{ID: "daily", Function: adapter.FunctionCall{
		Name: taskManageToolName, Arguments: `{"action":"create","name":"Brief","goal":"Brief me","schedule":{"type":"cron","cron":"0 9 * * *"}}`,
	}})
	if result.Status != "success" || len(created) != 1 || created[0].Schedule.Timezone != "Asia/Tokyo" {
		t.Fatalf("create = %#v, requests %#v", result, created)
	}
}

func TestATaskFromANewConversationReportsBackToIt(t *testing.T) {
	created := []CreateTaskRequest{}
	mutations := []TaskMutationContext{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{createCalls: &created, createMutations: &mutations})
	run, event := taskRuntimeFixture(invocation.OriginManual)
	delete(event.Context, "conversation_id")
	event.ReplyTo = nil
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil || prepared.BindConversation == nil {
		t.Fatalf("Prepare() = %#v, %v", prepared, err)
	}
	conversationID := uuid.New()
	prepared.BindConversation(conversationID)
	result := prepared.Tools[2].Handler(t.Context(), adapter.ToolCall{ID: "here", Function: adapter.FunctionCall{
		Name: taskManageToolName, Arguments: `{"action":"create","name":"Stretch","goal":"Remind me to stretch","delivery_mode":"source_conversation"}`,
	}})
	if result.Status != "success" || len(created) != 1 || created[0].DeliveryMode != TaskDeliverySourceConversation ||
		mutations[0].SourceConversationID != conversationID.String() {
		t.Fatalf("create = %#v, requests %#v, mutations %#v", result, created, mutations)
	}
}
