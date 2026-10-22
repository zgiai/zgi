package digitalhuman

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A channel has no task approval card, so the proposal itself is written out:
// what the task will be, or what changes, as the console card shows it.

var channelTaskOperationLabels = map[string]string{
	"create": "创建任务", "update": "修改任务", "cancel": "取消任务", "run_now": "立即执行任务",
	"pause": "暂停任务", "resume": "恢复任务", "retry": "重试任务",
}

// channelTaskApprovalDetails renders a pending task approval for a channel,
// or "" when the approval carries nothing to show.
func channelTaskApprovalDetails(state map[string]interface{}) string {
	// The continuation keeps it on its approval form.
	approval, _ := state["task_approval"].(map[string]interface{})
	if len(approval) == 0 {
		form, _ := state["approval_form"].(map[string]interface{})
		approval, _ = form["task_approval"].(map[string]interface{})
	}
	operation := strings.TrimSpace(stringValue(approval["operation"]))
	after, _ := approval["task_after"].(map[string]interface{})
	before, _ := approval["task_before"].(map[string]interface{})
	name := strings.TrimSpace(stringValue(after["name"]))
	if name == "" {
		name = strings.TrimSpace(stringValue(before["name"]))
	}
	if name == "" {
		name = strings.TrimSpace(stringValue(state["task_name"]))
	}
	label := channelTaskOperationLabels[operation]
	if label == "" {
		label = "任务操作"
	}
	switch operation {
	case "create":
		if len(after) == 0 {
			return ""
		}
		lines := []string{"待确认：" + label}
		lines = appendDetail(lines, "名称", name)
		lines = appendDetail(lines, "目标", stringValue(after["goal"]))
		lines = appendDetail(lines, "执行计划", channelTaskScheduleText(after["schedule"]))
		if schedule, _ := after["schedule"].(map[string]interface{}); stringValue(schedule["type"]) != "once" {
			lines = appendDetail(lines, "下次执行", channelTaskTimeText(after["next_run_at"], scheduleTimezone(after["schedule"])))
		}
		if criteria := channelTaskCriteria(after["success_criteria"]); len(criteria) > 0 {
			lines = append(lines, "成功标准：")
			for index, item := range criteria {
				lines = append(lines, fmt.Sprintf("%d. %s", index+1, item))
			}
		}
		lines = appendDetail(lines, "结果发送到", channelTaskDeliveryText(stringValue(after["delivery_mode"])))
		if priority := stringValue(after["priority"]); priority != "" && priority != string(TaskPriorityNormal) {
			lines = appendDetail(lines, "优先级", channelTaskPriorityText(priority))
		}
		return strings.Join(lines, "\n")
	case "update":
		changes, _ := approval["changes"].([]interface{})
		if len(changes) == 0 {
			return ""
		}
		lines := []string{"待确认：" + label + quoteName(name)}
		for _, raw := range sortedChanges(changes) {
			field := stringValue(raw["field"])
			label := strings.TrimSpace(stringValue(raw["label"]))
			if label == "" {
				label = field
			}
			lines = append(lines, label+"："+channelTaskFieldText(field, raw["before"], after)+" → "+channelTaskFieldText(field, raw["after"], after))
		}
		return strings.Join(lines, "\n")
	default:
		if name == "" {
			return ""
		}
		return "待确认：" + label + quoteName(name)
	}
}

func appendDetail(lines []string, label, value string) []string {
	if value = strings.TrimSpace(value); value == "" {
		return lines
	}
	return append(lines, label+"："+value)
}

func quoteName(name string) string {
	if name == "" {
		return ""
	}
	return "「" + name + "」"
}

// sortedChanges keeps a stable, readable order for changed fields.
func sortedChanges(changes []interface{}) []map[string]interface{} {
	order := map[string]int{"name": 0, "goal": 1, "schedule": 2, "next_run_at": 3, "success_criteria": 4, "delivery_mode": 5, "priority": 6}
	items := make([]map[string]interface{}, 0, len(changes))
	for _, raw := range changes {
		if item, ok := raw.(map[string]interface{}); ok {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, ok := order[stringValue(items[i]["field"])]
		if !ok {
			left = 99
		}
		right, ok := order[stringValue(items[j]["field"])]
		if !ok {
			right = 99
		}
		return left < right
	})
	return items
}

func channelTaskFieldText(field string, value interface{}, task map[string]interface{}) string {
	var text string
	switch field {
	case "schedule":
		text = channelTaskScheduleText(value)
	case "next_run_at":
		text = channelTaskTimeText(value, scheduleTimezone(task["schedule"]))
	case "success_criteria":
		text = strings.Join(channelTaskCriteria(value), "；")
	case "delivery_mode":
		text = channelTaskDeliveryText(stringValue(value))
	case "priority":
		text = channelTaskPriorityText(stringValue(value))
	case "execution_timeout_seconds":
		if seconds := numberValue(value); seconds > 0 {
			text = strconv.Itoa(seconds) + " 秒"
		}
	case "lifecycle_status":
		text = map[string]string{"active": "生效中", "paused": "已暂停", "archived": "已归档"}[stringValue(value)]
	default:
		if number := numberValue(value); number > 0 {
			text = strconv.Itoa(number)
		} else {
			text = strings.TrimSpace(stringValue(value))
		}
	}
	if text == "" {
		return "（无）"
	}
	return text
}

func numberValue(value interface{}) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	}
	return 0
}

func channelTaskCriteria(value interface{}) []string {
	raw, _ := value.([]interface{})
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		if text := strings.TrimSpace(stringValue(item)); text != "" {
			items = append(items, text)
		}
	}
	return items
}

func channelTaskDeliveryText(mode string) string {
	switch TaskDeliveryMode(mode) {
	case TaskDeliverySourceConversation:
		return "当前会话"
	case TaskDeliveryTaskOnly:
		return "仅保存在任务中"
	}
	return ""
}

func channelTaskPriorityText(priority string) string {
	return map[string]string{"low": "低", "normal": "普通", "high": "高", "urgent": "紧急"}[priority]
}

func scheduleTimezone(value interface{}) string {
	schedule, _ := value.(map[string]interface{})
	return strings.TrimSpace(stringValue(schedule["timezone"]))
}

// channelTaskTimeText shows a time in the task's time zone.
func channelTaskTimeText(value interface{}, timezone string) string {
	raw := strings.TrimSpace(stringValue(value))
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	if location, err := time.LoadLocation(timezone); timezone != "" && err == nil {
		parsed = parsed.In(location)
	}
	return parsed.Format("2006-01-02 15:04")
}

var channelWeekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// channelTaskScheduleText describes a task schedule in words: common cron
// patterns read naturally, anything else shows the expression itself.
func channelTaskScheduleText(value interface{}) string {
	schedule, _ := value.(map[string]interface{})
	timezone := strings.TrimSpace(stringValue(schedule["timezone"]))
	suffix := ""
	if timezone != "" {
		suffix = "（" + timezone + "）"
	}
	switch stringValue(schedule["type"]) {
	case "manual":
		return "手动执行"
	case "once":
		if at := channelTaskTimeText(schedule["run_at"], timezone); at != "" {
			return "单次，" + at + suffix
		}
		return "单次执行"
	case "cron":
		expression := strings.TrimSpace(stringValue(schedule["cron"]))
		if described := describeCron(expression); described != "" {
			return described + suffix
		}
		if expression != "" {
			return "按周期「" + expression + "」" + suffix
		}
	}
	return ""
}

func describeCron(expression string) string {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return ""
	}
	minute, hourField, day, month, weekday := fields[0], fields[1], fields[2], fields[3], fields[4]
	m, errM := strconv.Atoi(minute)
	h, errH := strconv.Atoi(hourField)
	if errM != nil || errH != nil || m < 0 || m > 59 || h < 0 || h > 23 || month != "*" {
		return ""
	}
	clock := fmt.Sprintf("%02d:%02d", h, m)
	switch {
	case day == "*" && weekday == "*":
		return "每天 " + clock
	case day == "*" && (weekday == "1-5" || weekday == "MON-FRI"):
		return "每个工作日 " + clock
	case day == "*" && (weekday == "0,6" || weekday == "6,0" || weekday == "SAT,SUN"):
		return "每个周末 " + clock
	case day == "*":
		names := make([]string, 0, 7)
		for _, part := range strings.Split(weekday, ",") {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index > 7 {
				return ""
			}
			names = append(names, channelWeekdays[index%7])
		}
		return "每" + strings.Join(names, "、") + " " + clock
	case weekday == "*":
		if d, err := strconv.Atoi(day); err == nil && d >= 1 && d <= 31 {
			return fmt.Sprintf("每月 %d 日 %s", d, clock)
		}
	}
	return ""
}
