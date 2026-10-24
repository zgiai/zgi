package digitalhuman

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// A channel has no task approval card, so the proposal is asked the way a
// person would ask it: one question, the time said as people say it, and
// only what is not obvious added below it.

var channelTaskOperationVerbs = map[string]string{
	"cancel": "取消", "run_now": "现在执行一次", "pause": "暂停", "resume": "恢复", "retry": "重新执行", "archive": "归档",
}

// channelTaskFieldNames name a task's fields in a question.
var channelTaskFieldNames = map[string]string{
	"name": "名称", "goal": "内容", "schedule": "时间", "success_criteria": "要求", "priority": "优先级",
	"delivery_mode": "结果去向", "execution_timeout_seconds": "超时时间", "max_attempts": "最多尝试次数",
	"lifecycle_status": "状态", "report_review": "验收方式",
}

// channelTaskApprovalDetails asks for a pending task approval in a channel,
// or "" when the approval carries nothing to show.
func channelTaskApprovalDetails(state map[string]interface{}) string {
	return channelTaskApprovalQuestion(state, time.Now())
}

func channelTaskApprovalQuestion(state map[string]interface{}, now time.Time) string {
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
	switch operation {
	case "create":
		if len(after) == 0 {
			return ""
		}
		return channelTaskCreateQuestion(name, after, now)
	case "update":
		changes, _ := approval["changes"].([]interface{})
		if len(changes) == 0 {
			return ""
		}
		return channelTaskUpdateQuestion(name, after, sortedChanges(changes), now)
	default:
		verb := channelTaskOperationVerbs[operation]
		if name == "" || verb == "" {
			return ""
		}
		return "要" + verb + "「" + name + "」吗？"
	}
}

func channelTaskCreateQuestion(name string, task map[string]interface{}, now time.Time) string {
	when := channelTaskScheduleWords(task["schedule"], now)
	goal := strings.TrimRight(strings.TrimSpace(stringValue(task["goal"])), "。.!！")
	lines := []string{}
	// A short goal without a time of its own reads as one question: 要我今天
	// 下午 3:43 提醒丁康源点外卖吗？ Anything else names the task first.
	if when != "" && goal != "" && utf8.RuneCountInString(goal) <= 40 && !strings.ContainsAny(goal, "0123456789") {
		lines = append(lines, "要我"+beforeWord(when)+goal+"吗？")
	} else {
		lines = append(lines, "要建任务「"+firstNonEmptyString(name, goal)+"」吗？")
		lines = appendDetail(lines, "时间", when)
		if goal != name {
			lines = appendDetail(lines, "内容", goal)
		}
	}
	if criteria := channelTaskCriteria(task["success_criteria"]); len(criteria) > 0 {
		lines = append(lines, "要求："+strings.Join(criteria, "；"))
	}
	if TaskDeliveryMode(stringValue(task["delivery_mode"])) == TaskDeliveryTaskOnly {
		lines = append(lines, "结果只保存在任务里，不发到这里。")
	}
	if priority := stringValue(task["priority"]); priority != "" && priority != string(TaskPriorityNormal) {
		lines = appendDetail(lines, "优先级", channelTaskPriorityText(priority))
	}
	return strings.Join(lines, "\n")
}

func channelTaskUpdateQuestion(name string, task map[string]interface{}, changes []map[string]interface{}, now time.Time) string {
	visible := make([]map[string]interface{}, 0, len(changes))
	for _, change := range changes {
		// The next run follows from the schedule; saying both repeats it.
		if field := stringValue(change["field"]); field != "next_run_at" {
			visible = append(visible, change)
		}
	}
	if len(visible) == 0 {
		visible = changes
	}
	subject := ""
	if name != "" {
		subject = "「" + name + "」的"
	}
	if len(visible) == 1 {
		change := visible[0]
		field := stringValue(change["field"])
		return "要把" + subject + channelTaskFieldName(change) + "从" + channelTaskFieldText(field, change["before"], task, now) +
			"改成" + beforeWord(channelTaskFieldText(field, change["after"], task, now)) + "吗？"
	}
	lines := []string{"要这样修改" + strings.TrimSuffix(subject, "的") + "吗？"}
	for _, change := range visible {
		field := stringValue(change["field"])
		lines = append(lines, channelTaskFieldName(change)+"："+channelTaskFieldText(field, change["before"], task, now)+" → "+channelTaskFieldText(field, change["after"], task, now))
	}
	return strings.Join(lines, "\n")
}

func channelTaskFieldName(change map[string]interface{}) string {
	if name := channelTaskFieldNames[stringValue(change["field"])]; name != "" {
		return name
	}
	if label := strings.TrimSpace(stringValue(change["label"])); label != "" {
		return label
	}
	return stringValue(change["field"])
}

// beforeWord spaces a time that ends in a digit from the word after it:
// 下午 3:43 提醒, but 下午 3 点提醒.
func beforeWord(text string) string {
	if strings.IndexAny(text[len(text)-min(len(text), 1):], "0123456789") == 0 {
		return text + " "
	}
	return text
}

func appendDetail(lines []string, label, value string) []string {
	if value = strings.TrimSpace(value); value == "" {
		return lines
	}
	return append(lines, label+"："+value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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

func channelTaskFieldText(field string, value interface{}, task map[string]interface{}, now time.Time) string {
	var text string
	switch field {
	case "schedule":
		text = channelTaskScheduleWords(value, now)
	case "next_run_at":
		text = channelTaskTimeWords(value, scheduleTimezone(task["schedule"]), now)
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
	case "report_review":
		text = map[string]string{"none": "无需验收", "exceptions": "有问题时处理", "required": "需要验收"}[stringValue(value)]
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
		return "发到这里"
	case TaskDeliveryTaskOnly:
		return "只保存在任务里"
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

// channelTaskTimeWords says a stored time the way people do, in the task's
// timezone.
func channelTaskTimeWords(value interface{}, timezone string, now time.Time) string {
	raw := strings.TrimSpace(stringValue(value))
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	return humanMoment(parsed, now, loadRunLocation(timezone)) + timezoneNote(timezone)
}

// channelTaskScheduleWords says when a task runs: 今天下午 3:43, 每个工作日
// 早上 9 点, or the cron expression itself when it has no common reading.
func channelTaskScheduleWords(value interface{}, now time.Time) string {
	schedule, _ := value.(map[string]interface{})
	timezone := strings.TrimSpace(stringValue(schedule["timezone"]))
	switch stringValue(schedule["type"]) {
	case "manual":
		return "需要时"
	case "once":
		if at := channelTaskTimeWords(schedule["run_at"], timezone, now); at != "" {
			return at
		}
		return ""
	case "cron":
		expression := strings.TrimSpace(stringValue(schedule["cron"]))
		if described := describeCron(expression); described != "" {
			return described + timezoneNote(timezone)
		}
		if expression != "" {
			return "按周期「" + expression + "」" + timezoneNote(timezone)
		}
	}
	return ""
}

// timezoneNote names a task's timezone only when it is not the one people
// here assume.
func timezoneNote(timezone string) string {
	if timezone == "" || timezone == defaultRunTimezone {
		return ""
	}
	return "（" + timezone + "）"
}

var channelWeekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

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
	clock := humanClock(h, m)
	switch {
	case day == "*" && weekday == "*":
		return "每天" + clock
	case day == "*" && (weekday == "1-5" || weekday == "MON-FRI"):
		return "每个工作日" + clock
	case day == "*" && (weekday == "0,6" || weekday == "6,0" || weekday == "SAT,SUN"):
		return "每个周末" + clock
	case day == "*":
		names := make([]string, 0, 7)
		for _, part := range strings.Split(weekday, ",") {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index > 7 {
				return ""
			}
			names = append(names, channelWeekdays[index%7])
		}
		return "每" + strings.Join(names, "、") + clock
	case weekday == "*":
		if d, err := strconv.Atoi(day); err == nil && d >= 1 && d <= 31 {
			return fmt.Sprintf("每月 %d 日%s", d, clock)
		}
	}
	return ""
}

// humanClock says a time of day: 早上 9 点, 下午 3:43, 中午 12 点.
func humanClock(hour, minute int) string {
	period := "晚上"
	switch {
	case hour < 6:
		period = "凌晨"
	case hour < 10:
		period = "早上"
	case hour < 12:
		period = "上午"
	case hour == 12:
		period = "中午"
	case hour < 18:
		period = "下午"
	}
	display := hour
	if hour > 12 {
		display = hour - 12
	}
	if minute == 0 {
		return fmt.Sprintf("%s %d 点", period, display)
	}
	return fmt.Sprintf("%s %d:%02d", period, display, minute)
}

// humanMoment says a moment relative to now: 今天下午 3:43, 明天早上 9 点,
// 10 月 30 日（周五）下午 5 点, with the year only when it is not this one.
func humanMoment(at, now time.Time, location *time.Location) string {
	at, now = at.In(location), now.In(location)
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, location) }
	clock := humanClock(at.Hour(), at.Minute())
	switch int(day(at).Sub(day(now)).Hours() / 24) {
	case 0:
		return "今天" + clock
	case 1:
		return "明天" + clock
	case 2:
		return "后天" + clock
	}
	date := fmt.Sprintf("%d 月 %d 日（%s）", int(at.Month()), at.Day(), channelWeekdays[int(at.Weekday())])
	if at.Year() != now.Year() {
		date = fmt.Sprintf("%d 年 ", at.Year()) + date
	}
	return date + clock
}
