package digitalhuman

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type taskRuntimeService struct {
	TaskHandlerService
	accesses        *[]TaskAccessContext
	statusCalls     *[]TaskWorkStatus
	createCalls     *[]CreateTaskRequest
	createMutations *[]TaskMutationContext
	listQueries     *[]TaskListQuery
	listResult      *TaskListResult
}

func (s taskRuntimeService) SetTaskWorkStatus(_ context.Context, access TaskAccessContext, taskID uuid.UUID, status TaskWorkStatus, revision int64, _ TaskMutationContext) (*Task, error) {
	if s.accesses != nil {
		*s.accesses = append(*s.accesses, access)
	}
	if s.statusCalls != nil {
		*s.statusCalls = append(*s.statusCalls, status)
	}
	return &Task{ID: taskID, WorkStatus: status, Revision: revision + 1}, nil
}

func (s taskRuntimeService) CreateTask(_ context.Context, access TaskAccessContext, req CreateTaskRequest, mutation TaskMutationContext) (*Task, error) {
	if s.accesses != nil {
		*s.accesses = append(*s.accesses, access)
	}
	if s.createCalls != nil {
		*s.createCalls = append(*s.createCalls, req)
	}
	if s.createMutations != nil {
		*s.createMutations = append(*s.createMutations, mutation)
	}
	status := TaskWorkStatusPending
	if mutation.Proposed {
		status = TaskWorkStatusProposed
	}
	return &Task{ID: uuid.New(), DigitalHumanID: access.DigitalHumanID, OwnerKind: access.Owner.Kind, Name: req.Name, Goal: req.Goal, WorkStatus: status, Revision: 1}, nil
}

func (s taskRuntimeService) ListTasks(_ context.Context, access TaskAccessContext, query TaskListQuery) (*TaskListResult, error) {
	if s.accesses != nil {
		*s.accesses = append(*s.accesses, access)
	}
	if s.listQueries != nil {
		*s.listQueries = append(*s.listQueries, query)
	}
	if s.listResult != nil {
		return s.listResult, nil
	}
	return &TaskListResult{}, nil
}

func TestTaskRuntimeChannelIsWritableByDefault(t *testing.T) {
	run, event := taskRuntimeFixture(invocation.OriginChannel)
	provider := NewTaskRuntimeProvider(taskRuntimeService{})
	preparation, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(preparation.Tools) != 3 || !strings.Contains(preparation.SystemPromptAddendum, "proposed task") || !strings.Contains(preparation.SystemPromptAddendum, "current group conversation") {
		t.Fatalf("channel preparation = %#v", preparation)
	}
	for _, tool := range preparation.Tools {
		encoded, err := json.Marshal(tool.Definition)
		if err != nil {
			t.Fatalf("marshal tool definition: %v", err)
		}
		definition := string(encoded)
		for _, forbidden := range []string{"organization_id", "digital_human_id", "installation_id", "conversation_id", "recipient_id"} {
			if strings.Contains(definition, forbidden) {
				t.Fatalf("tool %s exposes trusted boundary field %q: %s", tool.Definition.Function.Name, forbidden, definition)
			}
		}
	}
}

func TestTaskRuntimeManualAndCurrentScheduleAreWritable(t *testing.T) {
	provider := NewTaskRuntimeProvider(taskRuntimeService{})
	run, manual := taskRuntimeFixture(invocation.OriginManual)
	prepared, err := provider.Prepare(t.Context(), run, manual)
	if err != nil || len(prepared.Tools) != 3 {
		t.Fatalf("manual Prepare() = %#v, %v", prepared, err)
	}

	scheduled := manual
	scheduled.Origin = invocation.Origin{Kind: invocation.OriginSchedule}
	scheduled.Sender = invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"}
	scheduled.Context = map[string]any{"task_id": uuid.NewString()}
	prepared, err = provider.Prepare(t.Context(), run, scheduled)
	if err != nil || len(prepared.Tools) != 3 {
		t.Fatalf("scheduled Prepare() = %#v, %v", prepared, err)
	}
}

func TestScheduledTaskRuntimeMayOnlyMarkCurrentTaskWaitingOrBlocked(t *testing.T) {
	statuses := []TaskWorkStatus{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{statusCalls: &statuses})
	run, scheduled := taskRuntimeFixture(invocation.OriginSchedule)
	taskID := uuid.New()
	scheduled.Context = map[string]any{"task_id": taskID.String()}
	prepared, err := provider.Prepare(t.Context(), run, scheduled)
	if err != nil || len(prepared.Tools) != 3 || !strings.Contains(prepared.SystemPromptAddendum, "scheduled execution") {
		t.Fatalf("scheduled Prepare() = %#v, %v", prepared, err)
	}
	manage := prepared.Tools[2]
	propose := manage.Handler(t.Context(), adapter.ToolCall{ID: "propose", Function: adapter.FunctionCall{
		Name: taskManageToolName, Arguments: `{"action":"propose","name":"Another task","goal":"Do more work"}`,
	}})
	if propose.Status != "failed" {
		t.Fatalf("scheduled propose result = %#v", propose)
	}
	complete := manage.Handler(t.Context(), adapter.ToolCall{ID: "complete", Function: adapter.FunctionCall{
		Name: taskManageToolName, Arguments: `{"action":"set_status","task_id":"` + taskID.String() + `","expected_revision":1,"status":"completed"}`,
	}})
	if complete.Status != "failed" || len(statuses) != 0 {
		t.Fatalf("scheduled complete result = %#v, statuses = %v", complete, statuses)
	}
	waiting := manage.Handler(t.Context(), adapter.ToolCall{ID: "waiting", Function: adapter.FunctionCall{
		Name: taskManageToolName, Arguments: `{"action":"set_status","task_id":"` + taskID.String() + `","expected_revision":1,"status":"waiting"}`,
	}})
	if waiting.Status != "success" || len(statuses) != 1 || statuses[0] != TaskWorkStatusWaiting {
		t.Fatalf("scheduled waiting result = %#v, statuses = %v", waiting, statuses)
	}
}

func TestTaskRuntimeCreatesExplicitRequestWithoutProposalRoundTrip(t *testing.T) {
	created := []CreateTaskRequest{}
	mutations := []TaskMutationContext{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{createCalls: &created, createMutations: &mutations})
	run, event := taskRuntimeFixture(invocation.OriginChannel)
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	manage := prepared.Tools[2]
	result := manage.Handler(t.Context(), adapter.ToolCall{ID: "create-reminder", Function: adapter.FunctionCall{
		Name:      taskManageToolName,
		Arguments: `{"action":"create","name":"Drink water","goal":"Remind the user to drink water","schedule":{"type":"once","run_at":"2026-08-25T12:30:00Z","timezone":"Asia/Shanghai"}}`,
	}})
	if result.Status != "success" || len(created) != 1 || len(mutations) != 1 {
		t.Fatalf("create result = %#v, requests = %#v, mutations = %#v", result, created, mutations)
	}
	if mutations[0].Proposed {
		t.Fatal("explicit create was persisted as a proposal")
	}
	if mutations[0].SourceConversationType != "group" {
		t.Fatalf("create source conversation type = %q, want group", mutations[0].SourceConversationType)
	}
	data, ok := result.Result["data"].(map[string]any)
	if !ok || data["status"] != "committed" {
		t.Fatalf("create result data = %#v", result.Result)
	}
}

func TestTaskRuntimeKeepsInferredWorkAsProposal(t *testing.T) {
	mutations := []TaskMutationContext{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{createMutations: &mutations})
	run, event := taskRuntimeFixture(invocation.OriginManual)
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	result := prepared.Tools[2].Handler(t.Context(), adapter.ToolCall{ID: "propose-follow-up", Function: adapter.FunctionCall{
		Name: taskManageToolName, Arguments: `{"action":"propose","name":"Follow up","goal":"Consider following up next week"}`,
	}})
	if result.Status != "success" || len(mutations) != 1 || !mutations[0].Proposed {
		t.Fatalf("proposal result = %#v, mutations = %#v", result, mutations)
	}
}

func TestTaskRuntimeListDefaultsToOpenTasks(t *testing.T) {
	queries := []TaskListQuery{}
	item := &Task{ID: uuid.New(), Name: "Follow up", Goal: "Contact the customer", WorkStatus: TaskWorkStatusPending, Priority: TaskPriorityHigh, Revision: 4, UpdatedAt: time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)}
	provider := NewTaskRuntimeProvider(taskRuntimeService{listQueries: &queries, listResult: &TaskListResult{Items: []*Task{item}, Total: 1}})
	run, event := taskRuntimeFixture(invocation.OriginManual)
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	result := prepared.Tools[0].Handler(t.Context(), adapter.ToolCall{ID: "list-open", Function: adapter.FunctionCall{Name: taskListToolName, Arguments: `{}`}})
	if result.Status != "success" || len(queries) != 1 {
		t.Fatalf("list result = %#v, queries = %#v", result, queries)
	}
	wantStatuses := []TaskWorkStatus{TaskWorkStatusProposed, TaskWorkStatusPending, TaskWorkStatusInProgress, TaskWorkStatusWaiting, TaskWorkStatusBlocked}
	if !slices.Equal(queries[0].Statuses, wantStatuses) || queries[0].PageSize != taskListDefaultLimit || queries[0].UpdatedAfter != nil {
		t.Fatalf("open query = %#v, want statuses %#v and limit %d", queries[0], wantStatuses, taskListDefaultLimit)
	}
	data := result.Result["data"].(map[string]any)
	if data["view"] != taskListViewOpen {
		t.Fatalf("list view = %#v, want %q", data["view"], taskListViewOpen)
	}
	scope, ok := data["scope"].(taskListToolScope)
	if !ok || scope.Visibility != "current_user" {
		t.Fatalf("manual list scope = %#v, want current user tasks", data["scope"])
	}
}

func TestTaskRuntimeChannelListDisclosesConversationScope(t *testing.T) {
	queries := []TaskListQuery{}
	accesses := []TaskAccessContext{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{listQueries: &queries, accesses: &accesses})
	run, event := taskRuntimeFixture(invocation.OriginChannel)
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	result := prepared.Tools[0].Handler(t.Context(), adapter.ToolCall{ID: "list-channel", Function: adapter.FunctionCall{Name: taskListToolName, Arguments: `{}`}})
	if result.Status != "success" || len(queries) != 1 || len(accesses) != 1 || accesses[0].Owner.Kind != TaskOwnerConversation || accesses[0].Owner.ConversationID == uuid.Nil {
		t.Fatalf("channel list result = %#v, queries = %#v", result, queries)
	}
	data := result.Result["data"].(map[string]any)
	scope, ok := data["scope"].(taskListToolScope)
	if !ok || scope.Visibility != "current_conversation" || scope.ConversationType != "group" {
		t.Fatalf("channel list scope = %#v, want current group conversation", data["scope"])
	}
}

func TestTaskRuntimeCompletedHistoryIsExplicitAndBounded(t *testing.T) {
	queries := []TaskListQuery{}
	provider := NewTaskRuntimeProvider(taskRuntimeService{listQueries: &queries})
	provider.now = func() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) }
	run, event := taskRuntimeFixture(invocation.OriginManual)
	prepared, err := provider.Prepare(t.Context(), run, event)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	list := prepared.Tools[0]
	encoded, err := json.Marshal(list.Definition)
	if err != nil {
		t.Fatalf("marshal list definition: %v", err)
	}
	if strings.Contains(string(encoded), `"status"`) || !strings.Contains(string(encoded), `"view"`) {
		t.Fatalf("list definition must expose the new view contract: %s", encoded)
	}
	result := list.Handler(t.Context(), adapter.ToolCall{ID: "list-history", Function: adapter.FunctionCall{Name: taskListToolName, Arguments: `{"view":"completed","limit":5}`}})
	if result.Status != "success" || len(queries) != 1 {
		t.Fatalf("history result = %#v, queries = %#v", result, queries)
	}
	wantSince := provider.now().Add(-taskHistoryDefaultLookback)
	if !slices.Equal(queries[0].Statuses, []TaskWorkStatus{TaskWorkStatusCompleted}) || queries[0].PageSize != 5 || queries[0].UpdatedAfter == nil || !queries[0].UpdatedAfter.Equal(wantSince) {
		t.Fatalf("completed query = %#v, want completed since %s", queries[0], wantSince)
	}

	invalid := list.Handler(t.Context(), adapter.ToolCall{ID: "invalid-open-history", Function: adapter.FunctionCall{Name: taskListToolName, Arguments: `{"view":"open","history_since":"2026-08-01T00:00:00Z"}`}})
	if invalid.Status != "failed" || len(queries) != 1 {
		t.Fatalf("invalid history result = %#v, queries = %#v", invalid, queries)
	}
}

func TestTaskRuntimeFailureGivesModelSafeRecoveryAction(t *testing.T) {
	result := taskToolFailure("invalid_arguments", true)
	if !result.Recoverable || result.ModelResult["error_code"] != "invalid_arguments" ||
		!strings.Contains(result.ModelResult["next_action"].(string), "use create") {
		t.Fatalf("task failure = %#v", result)
	}
}

func TestTaskAccessForRunIsolatesAccountAndChannelOwners(t *testing.T) {
	manualRun, manualEvent := taskRuntimeFixture(invocation.OriginManual)
	manualAccess, err := taskAccessForRun(manualRun, manualEvent, taskBoundary(manualEvent))
	if err != nil || manualAccess.Owner.Kind != TaskOwnerUserSubject || manualAccess.Owner.Subject.ID != *manualRun.UserSubjectID {
		t.Fatalf("manual access = %#v, error = %v", manualAccess, err)
	}

	channelRun, channelEvent := taskRuntimeFixture(invocation.OriginChannel)
	first, err := taskAccessForRun(channelRun, channelEvent, taskBoundary(channelEvent))
	if err != nil || first.Owner.Kind != TaskOwnerConversation {
		t.Fatalf("channel access = %#v, error = %v", first, err)
	}
	second, err := taskAccessForRun(channelRun, channelEvent, taskBoundary(channelEvent))
	if err != nil || second.Owner.ConversationID != first.Owner.ConversationID {
		t.Fatalf("repeated channel owner = %#v, want %s", second.Owner, first.Owner.ConversationID)
	}
	otherEvent := channelEvent
	otherEvent.ReplyTo = &invocation.ReplyAddress{Provider: channelEvent.ReplyTo.Provider, InstallationID: channelEvent.ReplyTo.InstallationID, ConversationID: "other-chat"}
	other, err := taskAccessForRun(channelRun, otherEvent, taskBoundary(otherEvent))
	if err != nil || other.Owner.ConversationID == first.Owner.ConversationID {
		t.Fatalf("other channel owner = %#v, original = %s, error = %v", other.Owner, first.Owner.ConversationID, err)
	}
}

func taskRuntimeFixture(kind invocation.OriginKind) (*Run, invocation.Event) {
	organizationID, workspaceID, digitalHumanID, actorID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	run := &Run{ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: digitalHumanID, ActorID: &actorID, PrincipalType: string(invocation.PrincipalAccount), PrincipalID: &actorID, UserSubjectKind: "account", UserSubjectID: &actorID, Surface: SurfaceConsole}
	event := invocation.Event{
		Origin:  invocation.Origin{Kind: kind, Provider: "feishu", InstallationID: uuid.NewString()},
		Target:  runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID},
		Sender:  invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: actorID.String()},
		ReplyTo: &invocation.ReplyAddress{Provider: "feishu", InstallationID: uuid.NewString(), ConversationID: "chat-1"},
	}
	if kind == invocation.OriginChannel {
		run.UserSubjectKind, run.UserSubjectID, run.Surface = "", nil, SurfaceChannel
		event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "external-user"}
		event.Context = map[string]any{"conversation_type": "group"}
	} else if kind == invocation.OriginSchedule {
		run.Surface = SurfaceTask
		event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"}
	}
	return run, event
}
