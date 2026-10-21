package digitalhuman

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type occupancyTaskService struct {
	taskRuntimeService
	occupancyAccesses *[]TaskAccessContext
}

func (s occupancyTaskService) ListTaskOccupancy(_ context.Context, access TaskAccessContext, _ int) ([]TaskOccupancy, error) {
	*s.occupancyAccesses = append(*s.occupancyAccesses, access)
	next := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	return []TaskOccupancy{{NextRunAt: &next, WorkStatus: TaskWorkStatusPending, LifecycleStatus: "active", Priority: TaskPriorityHigh}}, nil
}

func prepareTaskListWithOccupancy(t *testing.T, kind invocation.OriginKind, surface string) (*TaskRuntimePreparation, map[string]any, []TaskAccessContext, []TaskAccessContext) {
	t.Helper()
	accesses, occupancy := []TaskAccessContext{}, []TaskAccessContext{}
	provider := NewTaskRuntimeProvider(occupancyTaskService{taskRuntimeService: taskRuntimeService{accesses: &accesses}, occupancyAccesses: &occupancy})
	run, event := taskRuntimeFixture(kind)
	if surface != "" {
		run.Surface = surface
	}
	event.Context = mergeContext(event.Context, map[string]any{"conversation_id": "1f6d2a7c-6a39-4c51-9a0f-8d6f5f1d7c11"})
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	result := prepared.Tools[0].Handler(t.Context(), adapter.ToolCall{ID: "list", Function: adapter.FunctionCall{Name: taskListToolName, Arguments: `{}`}})
	if result.Status != "success" {
		t.Fatalf("list result = %#v", result)
	}
	return prepared, result.Result["data"].(map[string]any), accesses, occupancy
}

func mergeContext(base, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func TestTaskRuntimeMemberSeesSharedTasksAndOccupancyWhenEnabled(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	prepared, data, accesses, occupancy := prepareTaskListWithOccupancy(t, invocation.OriginManual, "")
	if len(accesses) != 1 || accesses[0].Viewer.Mode != TaskViewMember || len(occupancy) != 1 {
		t.Fatalf("accesses = %#v, occupancy calls = %d", accesses, len(occupancy))
	}
	if scope := data["scope"].(taskListToolScope); scope.Visibility != "current_user_and_shared" {
		t.Fatalf("scope = %#v", scope)
	}
	if items, ok := data["other_open_work"].([]TaskOccupancy); !ok || len(items) != 1 {
		t.Fatalf("other_open_work = %#v", data["other_open_work"])
	}
	guidance := prepared.SystemPromptAddendum
	if !strings.Contains(guidance, "other_open_work") || strings.Contains(guidance, "current external conversation") || strings.Contains(guidance, "Shared DigitalHuman tasks and other users' private tasks are intentionally hidden") {
		t.Fatalf("member guidance = %q", guidance)
	}
}

func TestTaskRuntimeExternalUserNeverReceivesOccupancy(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	_, data, accesses, occupancy := prepareTaskListWithOccupancy(t, invocation.OriginAPI, SurfaceWebApp)
	if len(accesses) != 1 || accesses[0].Viewer.Mode != TaskViewExternal || len(occupancy) != 0 {
		t.Fatalf("accesses = %#v, occupancy calls = %d", accesses, len(occupancy))
	}
	if _, ok := data["other_open_work"]; ok {
		t.Fatalf("external list leaked occupancy: %#v", data)
	}
}

func TestTaskRuntimeChannelOccupancyKeepsConversationScope(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	prepared, data, accesses, occupancy := prepareTaskListWithOccupancy(t, invocation.OriginChannel, "")
	if len(accesses) != 1 || accesses[0].Viewer.Mode != TaskViewChannel || len(occupancy) != 1 {
		t.Fatalf("accesses = %#v, occupancy calls = %d", accesses, len(occupancy))
	}
	if scope := data["scope"].(taskListToolScope); scope.Visibility != "current_conversation" {
		t.Fatalf("scope = %#v", scope)
	}
	if !strings.Contains(prepared.SystemPromptAddendum, "current group conversation") || !strings.Contains(prepared.SystemPromptAddendum, "other_open_work") {
		t.Fatalf("channel guidance = %q", prepared.SystemPromptAddendum)
	}
}

func TestTaskRuntimeVisibilityDisabledKeepsLegacyList(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "")
	_, data, _, occupancy := prepareTaskListWithOccupancy(t, invocation.OriginManual, "")
	if _, ok := data["other_open_work"]; ok || len(occupancy) != 0 {
		t.Fatalf("legacy list returned occupancy: %#v", data)
	}
	if scope := data["scope"].(taskListToolScope); scope.Visibility == "current_user_and_shared" {
		t.Fatalf("legacy scope = %#v", scope)
	}
}

type readOnlyTaskService struct {
	taskRuntimeService
	canChange bool
}

func (s readOnlyTaskService) GetTask(_ context.Context, access TaskAccessContext, taskID uuid.UUID) (*Task, error) {
	return &Task{ID: taskID, DigitalHumanID: access.DigitalHumanID, OwnerKind: TaskOwnerDigitalHuman, Name: "shared", WorkStatus: TaskWorkStatusPending, LifecycleStatus: "active", Revision: 3, CanChange: s.canChange}, nil
}

func TestTaskRuntimeRefusesApprovalForReadOnlyTask(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	for _, canChange := range []bool{false, true} {
		provider := NewTaskRuntimeProvider(readOnlyTaskService{canChange: canChange})
		run, event := taskRuntimeFixture(invocation.OriginManual)
		prepared, err := provider.Prepare(t.Context(), run, event)
		if err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}
		result := prepared.Tools[2].Handler(t.Context(), adapter.ToolCall{ID: "cancel-shared", Function: adapter.FunctionCall{
			Name: taskManageToolName, Arguments: `{"action":"cancel","task_id":"` + uuid.NewString() + `","expected_revision":3}`,
		}})
		if canChange {
			if result.Status != "success" || result.PendingApproval == nil {
				t.Fatalf("changeable task result = %#v", result)
			}
			continue
		}
		if result.Status == "success" || result.PendingApproval != nil || !strings.Contains(fmt.Sprint(result.Result, result.Error), taskReadOnlyErrorCode) {
			t.Fatalf("read-only task result = %#v", result)
		}
	}
}
