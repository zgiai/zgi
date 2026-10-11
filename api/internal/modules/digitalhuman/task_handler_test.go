package digitalhuman

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

func TestExternalTaskMutationUsesSupportedActivityActorType(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/digital-human-webapps/test/tasks/test/actions/confirm", nil)
	req.Header.Set("Idempotency-Key", uuid.NewString())
	mutation := externalTaskMutation(ginCreateContext(req), "webapp", "account-actor")
	if mutation.ActorType != "external" {
		t.Fatalf("external task actor type = %q, want external", mutation.ActorType)
	}
	if mutation.Source != "webapp" {
		t.Fatalf("external task source = %q, want webapp", mutation.Source)
	}
}

// ginCreateContext keeps the unit test focused on mutation mapping without
// requiring a full router setup.
func ginCreateContext(req *http.Request) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req
	return ctx
}

func TestUserTaskAccessDerivesConsoleAccountSubject(t *testing.T) {
	accountID := uuid.New()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: accountID}
	access := userTaskAccess(scope, uuid.New())
	if access.Owner.Kind != TaskOwnerUserSubject || access.Owner.Subject.Kind != usersubject.Account || access.Owner.Subject.ID != accountID {
		t.Fatalf("user task owner = %#v, want account subject %s", access.Owner, accountID)
	}
}

func TestTaskApprovalActorTypeIsStableAcrossSurfaces(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope Scope
		want  string
	}{
		{name: "console", scope: Scope{Surface: SurfaceConsole}, want: "account"},
		{name: "authenticated account fallback", scope: Scope{PrincipalType: "account"}, want: "account"},
		{name: "api", scope: Scope{Surface: SurfaceAPI, PrincipalType: "service"}, want: "external"},
		{name: "webapp", scope: Scope{Surface: SurfaceWebApp, PrincipalType: "external_user"}, want: "external"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskApprovalActorType(tt.scope); got != tt.want {
				t.Fatalf("taskApprovalActorType(%+v) = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}

func TestTaskApprovalFormActionsExposeRevisionSeparately(t *testing.T) {
	actions := taskApprovalFormActions(true)
	if len(actions) != 3 {
		t.Fatalf("actions = %#v, want approve/revise/decline", actions)
	}
	if actions[0]["id"] != "approve" || actions[1]["id"] != "revise" || actions[2]["id"] != "decline" {
		t.Fatalf("action order = %#v, want approve/revise/decline", actions)
	}
	if got := taskApprovalFormActions(false); len(got) != 2 || got[0]["id"] != "approve" || got[1]["id"] != "decline" {
		t.Fatalf("actions without revision = %#v, want approve/decline", got)
	}
}

func TestTaskApprovalDiffDoesNotApplySupplementDirectly(t *testing.T) {
	task := &Task{Name: "每日汇报", Goal: "汇报小米股价", OwnerKind: TaskOwnerUserSubject}
	_, after, changes := taskApprovalDiff(task, map[string]interface{}{
		"goal":       "汇报小米和腾讯股价",
		"supplement": "改成工作日早上九点",
	}, "update")
	if got := after["goal"]; got != "汇报小米和腾讯股价" {
		t.Fatalf("task goal after approval = %#v, want model proposal without supplement", got)
	}
	for _, change := range changes {
		if change["field"] == "supplement" || strings.Contains(fmt.Sprint(change["after"]), "补充要求") {
			t.Fatalf("supplement leaked into task diff: %#v", changes)
		}
	}
}

func TestTaskApprovalDiffShowsLifecycleTransition(t *testing.T) {
	task := &Task{
		Name:            "每日汇报",
		Goal:            "汇报小米股价",
		OwnerKind:       TaskOwnerUserSubject,
		WorkStatus:      TaskWorkStatusPending,
		LifecycleStatus: "paused",
	}
	_, after, changes := taskApprovalDiff(task, taskApprovalRequestedInputs(manageTaskArguments{}, "resume"), "resume")
	if got := after["lifecycle_status"]; got != "active" {
		t.Fatalf("resume lifecycle status = %#v, want active", got)
	}
	for _, change := range changes {
		if change["field"] == "lifecycle_status" {
			if change["before"] != "paused" || change["after"] != "active" {
				t.Fatalf("lifecycle change = %#v, want paused -> active", change)
			}
			return
		}
	}
	t.Fatalf("resume lifecycle change missing: %#v", changes)
}

func TestTaskApprovalDiffShowsBlockedWorkStatusRecovery(t *testing.T) {
	task := &Task{
		Name:            "每日汇报",
		Goal:            "汇报小米股价",
		OwnerKind:       TaskOwnerUserSubject,
		WorkStatus:      TaskWorkStatusBlocked,
		LifecycleStatus: "paused",
	}
	inputs := map[string]interface{}{"lifecycle_status": "active", "work_status": TaskWorkStatusPending}
	_, after, changes := taskApprovalDiff(task, inputs, "resume")
	if got := after["work_status"]; got != TaskWorkStatusPending {
		t.Fatalf("resume work status = %#v, want pending", got)
	}
	for _, change := range changes {
		if change["field"] == "work_status" {
			if fmt.Sprint(change["before"]) != string(TaskWorkStatusBlocked) || fmt.Sprint(change["after"]) != string(TaskWorkStatusPending) {
				t.Fatalf("work status change = %#v, want blocked -> pending", change)
			}
			return
		}
	}
	t.Fatalf("resume work status change missing: %#v", changes)
}

func TestTaskApprovalRequestedInputsUseWorkStatusForLegacyStatusChanges(t *testing.T) {
	inputs := taskApprovalRequestedInputs(manageTaskArguments{Status: TaskWorkStatusWaiting}, "set_status")
	if inputs["work_status"] != TaskWorkStatusWaiting || inputs["status"] != TaskWorkStatusWaiting {
		t.Fatalf("status inputs = %#v, want both canonical and legacy keys", inputs)
	}
}

func TestTaskApprovalActionForPausedPendingStatusBecomesResume(t *testing.T) {
	task := &Task{LifecycleStatus: "paused"}
	action := taskApprovalActionForTask(manageTaskArguments{Action: "set_status", Status: TaskWorkStatusPending}, task)
	if action != "resume" {
		t.Fatalf("normalized approval action = %q, want resume", action)
	}
	if got := taskApprovalActionForTask(manageTaskArguments{Action: "set_status", Status: TaskWorkStatusWaiting}, task); got != "set_status" {
		t.Fatalf("waiting status action = %q, want set_status", got)
	}
}

func TestAugmentTaskApprovalRequestedInputsCombinesScheduleAndResume(t *testing.T) {
	run := &Run{Query: "恢复一下股票汇报，改为三分钟后给我单次汇报"}
	task := &Task{LifecycleStatus: "paused"}
	inputs := map[string]interface{}{"schedule": &TaskSchedule{Type: TaskScheduleOnce}}
	augmentTaskApprovalRequestedInputs(run, task, "schedule", inputs)
	if got := inputs["lifecycle_status"]; got != "active" {
		t.Fatalf("lifecycle_status = %#v, want active", got)
	}
}

func TestAugmentTaskApprovalRequestedInputsDoesNotInferResumeFromUnrelatedScheduleChange(t *testing.T) {
	run := &Run{Query: "把汇报改到明天早上九点"}
	task := &Task{LifecycleStatus: "paused"}
	inputs := map[string]interface{}{"schedule": &TaskSchedule{Type: TaskScheduleCron, Cron: "0 9 * * *"}}
	augmentTaskApprovalRequestedInputs(run, task, "schedule", inputs)
	if _, ok := inputs["lifecycle_status"]; ok {
		t.Fatalf("unexpected lifecycle_status for unrelated schedule change: %#v", inputs)
	}
}

func TestAugmentTaskApprovalRequestedInputsPreservesExplicitLifecycle(t *testing.T) {
	run := &Run{Query: "改时间"}
	task := &Task{LifecycleStatus: "paused"}
	inputs := map[string]interface{}{"lifecycle_status": "paused"}
	augmentTaskApprovalRequestedInputs(run, task, "update", inputs)
	if got := inputs["lifecycle_status"]; got != "paused" {
		t.Fatalf("explicit lifecycle_status = %#v, want paused", got)
	}
}

func TestAugmentTaskApprovalRequestedInputsIgnoresNegatedResume(t *testing.T) {
	run := &Run{Query: "改时间，但不要恢复任务"}
	task := &Task{LifecycleStatus: "paused"}
	inputs := map[string]interface{}{"schedule": &TaskSchedule{Type: TaskScheduleOnce}}
	augmentTaskApprovalRequestedInputs(run, task, "schedule", inputs)
	if _, ok := inputs["lifecycle_status"]; ok {
		t.Fatalf("unexpected lifecycle_status for negated resume request: %#v", inputs)
	}
}

func TestMergeTaskApprovalInputsKeepsModelProposalOnApproval(t *testing.T) {
	state := map[string]interface{}{
		"requested_inputs": map[string]interface{}{
			"name": "模型提出的任务",
			"goal": "模型提出的目标",
		},
	}
	merged := mergeTaskApprovalInputs(state, nil)
	if merged["name"] != "模型提出的任务" || merged["goal"] != "模型提出的目标" {
		t.Fatalf("merged approval inputs = %#v, want model proposal", merged)
	}
	if got := mergeTaskApprovalInputs(state, map[string]interface{}{"name": "用户直接改名"})["name"]; got != "用户直接改名" {
		t.Fatalf("explicit revision inputs = %#v, want submitted value for non-approval action", got)
	}
}

func TestTaskUpdateRequestFromApprovalInputsCarriesLifecycleStatus(t *testing.T) {
	request := taskUpdateRequestFromApprovalInputs(7, map[string]interface{}{
		"lifecycle_status": "active",
		"name":             "恢复后的任务",
	})
	if request.LifecycleStatus == nil || *request.LifecycleStatus != "active" {
		t.Fatalf("lifecycle status = %#v, want active", request.LifecycleStatus)
	}
	if request.Name == nil || *request.Name != "恢复后的任务" {
		t.Fatalf("name = %#v, want proposal name", request.Name)
	}
}
