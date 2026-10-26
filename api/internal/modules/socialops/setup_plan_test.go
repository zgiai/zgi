package socialops

import (
	"reflect"
	"strings"
	"testing"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestBuildSetupPlanUsesExistingCronContractAndBoundedTasks(t *testing.T) {
	cfg := validConfiguration("x-social", RequirementOptional)
	plan, err := BuildSetupPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 4 {
		t.Fatalf("task count = %d", len(plan.Tasks))
	}
	wantCrons := []string{cfg.Cadence.DailyContentCron, cfg.Cadence.InboxCron, cfg.Cadence.SevenDaySampleCron, cfg.Cadence.WeeklyReviewCron}
	for i, task := range plan.Tasks {
		if task.Request.Schedule.Type != digitalhuman.TaskScheduleCron || task.Request.Schedule.Cron != wantCrons[i] || task.Request.Schedule.Timezone != cfg.Timezone {
			t.Fatalf("task %d schedule = %+v", i, task.Request.Schedule)
		}
		if _, err := automationdefinition.CompileNextRunAt(automationmodel.AutomationScheduleTypeCron, task.Request.Schedule.Timezone, map[string]any{"cron_expr": task.Request.Schedule.Cron}); err != nil {
			t.Fatalf("task %d scheduler rejected plan: %v", i, err)
		}
		if task.Budget.MaxPages < 1 || task.Budget.MaxConnectorCalls < 1 || task.Budget.MaxRecords < 1 || task.Budget.MaxTokens < 1 || task.Budget.MaxSafeReadAttempts < 1 || task.Budget.MaxSafeReadAttempts > 3 || task.Request.ExecutionTimeoutSeconds < 30 || task.Request.ExecutionTimeoutSeconds > 1800 || task.Request.MaxAttempts != 1 {
			t.Fatalf("task %d is not bounded: %+v %+v", i, task.Budget, task.Request)
		}
		for _, ref := range []string{cfg.Scope.OwnerID.String(), cfg.Scope.EmployeeID.String(), cfg.EmployeeVersionID.String(), "configuration revision 7"} {
			if !strings.Contains(task.Request.Goal, ref) || !containsSubstring(task.Request.SuccessCriteria, ref) {
				t.Fatalf("task %d missing ref %q", i, ref)
			}
		}
		if len(task.ConsumesRecordKinds) == 0 {
			t.Fatalf("task %d has no persisted inputs", i)
		}
	}
}

func TestBuildSetupPlanCoversAllTenStagesAndWeeklyFeedback(t *testing.T) {
	plan, err := BuildSetupPlan(validConfiguration("x-social", RequirementOptional))
	if err != nil {
		t.Fatal(err)
	}
	covered := map[SOPStage]bool{}
	for _, task := range plan.Tasks {
		for _, stage := range task.Stages {
			covered[stage] = true
		}
	}
	for _, stage := range AllSOPStages() {
		if !covered[stage] {
			t.Errorf("stage %q is not covered", stage)
		}
	}
	weekly := plan.Tasks[3]
	wantUpdates := []RecordTarget{{Kind: "topic", Key: "next_topic"}, {Kind: "writing_rule", Key: "current"}}
	if !reflect.DeepEqual(weekly.UpdatesRecords, wantUpdates) {
		t.Fatalf("weekly updates = %#v", weekly.UpdatesRecords)
	}
	if !strings.Contains(weekly.Request.Goal, "next_topic") || !strings.Contains(weekly.Request.Goal, "writing_rule") {
		t.Fatalf("weekly goal = %q", weekly.Request.Goal)
	}
}

func TestBuildSetupPlanSubstitutesConfiguredProduct(t *testing.T) {
	one := validConfiguration("x-social", RequirementOptional)
	two := one
	two.Product = ProductConfiguration{Name: "Nimbus CRM", Description: "Customer relationship software"}
	first, err := BuildSetupPlan(one)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSetupPlan(two)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first.Tasks {
		if !strings.Contains(first.Tasks[i].Request.Goal, "Acme Analytics") || !strings.Contains(second.Tasks[i].Request.Goal, "Nimbus CRM") || strings.Contains(second.Tasks[i].Request.Goal, "Acme Analytics") {
			t.Fatalf("task %d did not substitute product:\n%s\n%s", i, first.Tasks[i].Request.Goal, second.Tasks[i].Request.Goal)
		}
		if first.Tasks[i].Key != second.Tasks[i].Key || !reflect.DeepEqual(first.Tasks[i].Stages, second.Tasks[i].Stages) || first.Tasks[i].Request.Schedule != second.Tasks[i].Request.Schedule {
			t.Fatalf("task %d structure changed with product", i)
		}
	}
}

func containsSubstring(values []string, substring string) bool {
	for _, value := range values {
		if strings.Contains(value, substring) {
			return true
		}
	}
	return false
}
