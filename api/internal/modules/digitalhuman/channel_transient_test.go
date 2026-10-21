package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func TestDecodeChannelTransientTask(t *testing.T) {
	runID := uuid.New()
	task := asynq.NewTask(ChannelTransientDeliveryTaskType, []byte(`{
		"run_id":"`+runID.String()+`",
		"kind":"progress",
		"delivery_key":"segment:1",
		"sequence":42,
		"text":"我先确认当前时间。"
	}`))
	request, err := decodeChannelTransientTask(task)
	if err != nil {
		t.Fatalf("decodeChannelTransientTask() error = %v", err)
	}
	if request.RunID != runID || request.Kind != channelruntime.DeliveryKindProgress ||
		request.DeliveryKey != "segment:1" || request.Sequence != 42 || request.Text != "我先确认当前时间。" {
		t.Fatalf("request = %#v", request)
	}
}

func TestDecodeChannelTransientTaskRejectsUnknownFields(t *testing.T) {
	task := asynq.NewTask(ChannelTransientDeliveryTaskType, []byte(`{
		"run_id":"`+uuid.NewString()+`",
		"kind":"progress",
		"delivery_key":"segment:1",
		"sequence":1,
		"text":"处理中",
		"persist":true
	}`))
	if _, err := decodeChannelTransientTask(task); err == nil {
		t.Fatal("decodeChannelTransientTask() error = nil, want rejection")
	}
}
