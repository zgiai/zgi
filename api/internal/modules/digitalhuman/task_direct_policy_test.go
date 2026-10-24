package digitalhuman

import (
	"testing"
)

func TestTaskChangesApplyAtOnceOnlyWhenLowRisk(t *testing.T) {
	console := taskRuntimeBoundary{source: "console"}
	direct := taskRuntimeBoundary{source: "channel:feishu", conversationType: "direct"}
	group := taskRuntimeBoundary{source: "channel:dingtalk", conversationType: "group"}
	api := taskRuntimeBoundary{source: "external-api"}
	once := TaskSchedule{Type: TaskScheduleOnce, Timezone: "Asia/Shanghai"}
	daily := TaskSchedule{Type: TaskScheduleCron, Cron: "0 9 * * 1-5", Timezone: "Asia/Shanghai"}
	hourly := TaskSchedule{Type: TaskScheduleCron, Cron: "0 * * * *", Timezone: "Asia/Shanghai"}
	twiceDaily := TaskSchedule{Type: TaskScheduleCron, Cron: "0 9,18 * * *", Timezone: "Asia/Shanghai"}

	create := func(boundary taskRuntimeBoundary, action string, schedule TaskSchedule) bool {
		return directTaskCreateAllowed(boundary, manageTaskArguments{Action: action, Schedule: &schedule})
	}
	change := func(boundary taskRuntimeBoundary, action string, schedule *TaskSchedule, current TaskSchedule) bool {
		return directTaskChangeAllowed(boundary, manageTaskArguments{Action: action, Schedule: schedule}, &Task{Schedule: current})
	}
	for name, tc := range map[string]struct {
		got, want bool
	}{
		"console one-off":       {create(console, "create", once), true},
		"console daily":         {create(console, "create", daily), true},
		"console hourly":        {create(console, "create", hourly), false},
		"twice a day":           {create(direct, "create", twiceDaily), false},
		"direct chat daily":     {create(direct, "create", daily), true},
		"group one-off":         {create(group, "create", once), true},
		"group daily":           {create(group, "create", daily), false},
		"api one-off":           {create(api, "create", once), false},
		"suggested, not asked":  {create(console, "propose", once), false},
		"manual task":           {directTaskCreateAllowed(console, manageTaskArguments{Action: "create"}), true},
		"edit a daily task":     {change(console, "update", nil, daily), true},
		"make it hourly":        {change(console, "schedule", &hourly, daily), false},
		"pause an hourly task":  {change(console, "pause", nil, hourly), true},
		"resume an hourly task": {change(console, "resume", nil, hourly), false},
		"cancel":                {change(console, "cancel", nil, once), false},
		"archive":               {change(console, "archive", nil, once), false},
		"group one-off run now": {change(group, "run_now", nil, once), true},
		"group recurring pause": {change(group, "pause", nil, daily), false},
		"group confirm":         {change(group, "confirm", nil, once), false},
		"direct chat confirm":   {change(direct, "confirm", nil, once), true},
		"api update":            {change(api, "update", nil, once), false},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: direct = %v, want %v", name, tc.got, tc.want)
		}
	}
	if !undoableTaskAction("pause") || undoableTaskAction("run_now") || undoableTaskAction("cancel") {
		t.Fatal("only changes that can be put back are undoable")
	}
	if _, ok := cronMinInterval(TaskSchedule{Type: TaskScheduleCron, Cron: "not a cron"}); ok {
		t.Fatal("an invalid cron has an interval")
	}
}
