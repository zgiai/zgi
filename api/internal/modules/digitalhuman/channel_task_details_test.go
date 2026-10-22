package digitalhuman

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// The continuation record of a real proposal, as the task tool stores it.
const channelTaskProposalFixture = `{"kind": "digital_human_task", "task_name": "工作日早9点喝水提醒", "approval_form": {"task_approval": {"changes": null, "operation": "create", "task_after": {
	"goal": "每个工作日（周一至周五）早上 9:00 向我发送一条喝水提醒，提示我及时补充水分。", "name": "工作日早9点喝水提醒", "priority": "normal",
	"schedule": {"cron": "0 9 * * 1-5", "type": "cron", "timezone": "Asia/Shanghai"}, "delivery_mode": "source_conversation",
	"success_criteria": ["每个工作日早上 9:00（Asia/Shanghai）准时生成一条喝水提醒", "提醒内容简短明确，包含喝水建议", "周末（周六、周日）不发送提醒"]}}}}`

func TestChannelTaskProposalShowsTheTask(t *testing.T) {
	var state map[string]interface{}
	if err := json.Unmarshal([]byte(channelTaskProposalFixture), &state); err != nil {
		t.Fatal(err)
	}
	text := channelWaitingPromptText(nil, runtimemodel.MessageStatusWaitingApproval, "我已为你准备好任务，请确认。",
		map[string]interface{}{"agent_workflow_continuation": state})
	for _, want := range []string{
		"待确认：创建任务", "名称：工作日早9点喝水提醒", "目标：每个工作日（周一至周五）早上 9:00",
		"执行计划：每个工作日 09:00（Asia/Shanghai）", "成功标准：\n1. 每个工作日早上 9:00", "3. 周末（周六、周日）不发送提醒",
		"结果发送到：当前会话", channelReplyTaskApproval,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt = %q\nwant it to contain %q", text, want)
		}
	}
}

func TestChannelTaskUpdateShowsWhatChanges(t *testing.T) {
	state := map[string]interface{}{"task_approval": map[string]interface{}{
		"operation":   "update",
		"task_before": map[string]interface{}{"name": "喝水提醒"},
		"task_after":  map[string]interface{}{"name": "喝水提醒", "schedule": map[string]interface{}{"type": "cron", "cron": "30 8 * * *", "timezone": "Asia/Shanghai"}},
		"changes": []interface{}{map[string]interface{}{
			"field": "schedule", "label": "执行计划",
			"before": map[string]interface{}{"type": "cron", "cron": "0 9 * * 1-5", "timezone": "Asia/Shanghai"},
			"after":  map[string]interface{}{"type": "cron", "cron": "30 8 * * *", "timezone": "Asia/Shanghai"},
		}},
	}}
	got := channelTaskApprovalDetails(state)
	want := "待确认：修改任务「喝水提醒」\n执行计划：每个工作日 09:00（Asia/Shanghai） → 每天 08:30（Asia/Shanghai）"
	if got != want {
		t.Fatalf("update details = %q, want %q", got, want)
	}
}

func TestDescribeCronReadsCommonSchedules(t *testing.T) {
	for expression, want := range map[string]string{
		"0 9 * * *": "每天 09:00", "0 9 * * 1-5": "每个工作日 09:00", "0 17 * * 5": "每周五 17:00",
		"0 10 * * 1,3": "每周一、周三 10:00", "0 8 1 * *": "每月 1 日 08:00", "*/5 * * * *": "",
	} {
		if got := describeCron(expression); got != want {
			t.Fatalf("describeCron(%q) = %q, want %q", expression, got, want)
		}
	}
}
