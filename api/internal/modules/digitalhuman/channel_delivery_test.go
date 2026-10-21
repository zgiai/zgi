package digitalhuman

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestDecorateScheduledTaskResultAddsTaskIdentity(t *testing.T) {
	event := invocation.Event{
		ID:     "scheduled-run",
		Type:   invocation.EventTypeSchedule,
		Origin: invocation.Origin{Kind: invocation.OriginSchedule},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "scheduler"},
		Context: map[string]any{
			"task_name": "每日销售简报",
		},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	got := decorateScheduledTaskResult(&Run{InvocationEvent: datatypes.JSON(encoded)}, "今天完成了数据整理。")
	want := "任务“每日销售简报”执行完成\n\n今天完成了数据整理。"
	if got != want {
		t.Fatalf("decorated result = %q, want %q", got, want)
	}
}

func TestDecorateScheduledTaskResultLeavesInteractiveRunUnchanged(t *testing.T) {
	event := invocation.Event{
		ID:      "channel-run",
		Type:    invocation.EventTypeChannelMessage,
		Origin:  invocation.Origin{Kind: invocation.OriginChannel},
		Target:  runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		Sender:  invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "user"},
		Context: map[string]any{"task_name": "不应添加前缀"},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	const summary = "交互回复"
	if got := decorateScheduledTaskResult(&Run{InvocationEvent: datatypes.JSON(encoded)}, summary); got != summary {
		t.Fatalf("interactive result = %q, want %q", got, summary)
	}
}

func TestChannelDeliveryLookupErrorSeparatesMissingTargetFromDatabaseFailure(t *testing.T) {
	missing := channelDeliveryLookupError("load channel installation for delivery", gorm.ErrRecordNotFound)
	if !errors.Is(missing, errChannelDeliveryTargetUnavailable) || !errors.Is(missing, gorm.ErrRecordNotFound) {
		t.Fatalf("missing target error = %v, want an unavailable target that keeps its cause", missing)
	}
	failure := channelDeliveryLookupError("load channel installation for delivery", errors.New("connection reset"))
	if errors.Is(failure, errChannelDeliveryTargetUnavailable) {
		t.Fatalf("database failure %v was classified as an unavailable target", failure)
	}
}
