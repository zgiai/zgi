package digitalhuman

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// The continuation record of a real proposal, as the task tool stores it.
const channelTaskProposalFixture = `{"kind": "digital_human_task", "task_name": "工作日早9点喝水提醒", "approval_form": {"task_approval": {"changes": null, "operation": "create", "task_after": {
	"goal": "每个工作日（周一至周五）早上 9:00 向我发送一条喝水提醒，提示我及时补充水分。", "name": "工作日早9点喝水提醒", "priority": "normal",
	"schedule": {"cron": "0 9 * * 1-5", "type": "cron", "timezone": "Asia/Shanghai"}, "delivery_mode": "source_conversation",
	"success_criteria": ["提醒内容简短明确，包含喝水建议"]}}}}`

func TestChannelTaskProposalAsksLikeAPerson(t *testing.T) {
	var state map[string]interface{}
	if err := json.Unmarshal([]byte(channelTaskProposalFixture), &state); err != nil {
		t.Fatal(err)
	}
	text := channelWaitingPromptText(nil, runtimemodel.MessageStatusWaitingApproval, "",
		map[string]interface{}{"agent_workflow_continuation": state})
	for _, want := range []string{
		"要建任务「工作日早9点喝水提醒」吗？", "时间：每个工作日早上 9 点", "内容：每个工作日（周一至周五）早上 9:00 向我发送一条喝水提醒",
		"要求：提醒内容简短明确", channelReplyTaskApproval,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt = %q\nwant it to contain %q", text, want)
		}
	}
	for _, unwanted := range []string{"待确认", "Asia/Shanghai", "结果发送到", "优先级"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("prompt = %q has %q", text, unwanted)
		}
	}
}

func TestAShortOneOffTaskIsOneQuestion(t *testing.T) {
	now := time.Date(2026, 10, 24, 7, 30, 0, 0, time.UTC)
	state := map[string]interface{}{"task_approval": map[string]interface{}{"operation": "create", "task_after": map[string]interface{}{
		"name": "点外卖提醒", "goal": "提醒丁康源点外卖。", "priority": "high",
		"schedule": map[string]interface{}{"type": "once", "run_at": "2026-10-24T07:43:00Z", "timezone": "Asia/Shanghai"},
	}}}
	if got, want := channelTaskApprovalQuestion(state, now), "要我今天下午 3:43 提醒丁康源点外卖吗？\n优先级：高"; got != want {
		t.Fatalf("question = %q, want %q", got, want)
	}
}

func TestChannelTaskUpdateAsksWhatChanges(t *testing.T) {
	state := map[string]interface{}{"task_approval": map[string]interface{}{
		"operation":   "update",
		"task_before": map[string]interface{}{"name": "喝水提醒"},
		"task_after":  map[string]interface{}{"name": "喝水提醒", "schedule": map[string]interface{}{"type": "cron", "cron": "30 8 * * *", "timezone": "Asia/Shanghai"}},
		"changes": []interface{}{
			map[string]interface{}{"field": "next_run_at", "label": "下次执行时间", "before": "2026-10-26T01:00:00Z", "after": "2026-10-25T00:30:00Z"},
			map[string]interface{}{
				"field": "schedule", "label": "执行计划",
				"before": map[string]interface{}{"type": "cron", "cron": "0 9 * * 1-5", "timezone": "Asia/Shanghai"},
				"after":  map[string]interface{}{"type": "cron", "cron": "30 8 * * *", "timezone": "Asia/Shanghai"},
			},
		},
	}}
	got := channelTaskApprovalDetails(state)
	want := "要把「喝水提醒」的时间从每个工作日早上 9 点改成每天早上 8:30 吗？"
	if got != want {
		t.Fatalf("update question = %q, want %q", got, want)
	}
	if got := channelTaskApprovalDetails(map[string]interface{}{"task_approval": map[string]interface{}{
		"operation": "cancel", "task_before": map[string]interface{}{"name": "喝水提醒"},
	}}); got != "要取消「喝水提醒」吗？" {
		t.Fatalf("cancel question = %q", got)
	}
}

func TestTimesReadTheWayPeopleSayThem(t *testing.T) {
	location := loadRunLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 24, 15, 0, 0, 0, location)
	for at, want := range map[time.Time]string{
		time.Date(2026, 10, 24, 15, 43, 0, 0, location): "今天下午 3:43",
		time.Date(2026, 10, 25, 9, 0, 0, 0, location):   "明天早上 9 点",
		time.Date(2026, 10, 26, 7, 30, 0, 0, location):  "后天早上 7:30",
		time.Date(2026, 10, 30, 12, 0, 0, 0, location):  "10 月 30 日（周五）中午 12 点",
		time.Date(2027, 1, 5, 20, 0, 0, 0, location):    "2027 年 1 月 5 日（周二）晚上 8 点",
	} {
		if got := humanMoment(at, now, location); got != want {
			t.Fatalf("humanMoment(%s) = %q, want %q", at, got, want)
		}
	}
	for expression, want := range map[string]string{
		"0 9 * * *": "每天早上 9 点", "0 9 * * 1-5": "每个工作日早上 9 点", "0 17 * * 5": "每周五下午 5 点",
		"0 10 * * 1,3": "每周一、周三上午 10 点", "0 8 1 * *": "每月 1 日早上 8 点", "*/5 * * * *": "",
	} {
		if got := describeCron(expression); got != want {
			t.Fatalf("describeCron(%q) = %q, want %q", expression, got, want)
		}
	}
	if timezoneNote("Asia/Shanghai") != "" || timezoneNote("Europe/Berlin") != "（Europe/Berlin）" {
		t.Fatal("only another timezone is named")
	}
}
