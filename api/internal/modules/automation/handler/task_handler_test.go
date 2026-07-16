package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationrepo "github.com/zgiai/zgi/api/internal/modules/automation/repository"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

func TestParseStatusesIgnoresAllSentinel(t *testing.T) {
	statuses := parseStatuses("all")
	if len(statuses) != 0 {
		t.Fatalf("expected all sentinel to disable status filtering, got %#v", statuses)
	}
}

func TestParseStatusesKeepsConcreteStatuses(t *testing.T) {
	statuses := parseStatuses("active, paused, all")
	expected := []automationmodel.AutomationTaskStatus{"active", "paused"}

	if len(statuses) != len(expected) {
		t.Fatalf("expected %d statuses, got %#v", len(expected), statuses)
	}
	for index, value := range expected {
		if statuses[index] != value {
			t.Fatalf("unexpected status at %d: got %q want %q", index, statuses[index], value)
		}
	}
}

func TestTaskReadRoutesRequireWorkspaceViewBeforeServiceLookup(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		call   func(*TaskHandler, *gin.Context)
	}{
		{
			name:   "get task",
			method: http.MethodGet,
			target: "/automations/tasks/task-1?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).GetTask,
		},
		{
			name:   "list tasks",
			method: http.MethodGet,
			target: "/automations/tasks?workspace_id=workspace-1",
			call:   (*TaskHandler).ListTasks,
		},
		{
			name:   "list task runs",
			method: http.MethodGet,
			target: "/automations/tasks/task-1/runs?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).ListTaskRuns,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &taskPermissionService{}
			actionRepo := &taskPermissionActionRepository{}
			runRepo := &taskPermissionRunRepository{}
			permissionChecker := &taskPermissionOrganizationService{allowed: false}
			handler := &TaskHandler{
				service:             service,
				actionRepo:          actionRepo,
				runRepo:             runRepo,
				organizationService: permissionChecker,
			}
			ctx, recorder := newTaskPermissionContext(tt.method, tt.target, "", tt.params)

			tt.call(handler, ctx)

			requireTaskPermissionDenied(t, recorder)
			if permissionChecker.lastPermission != workspacemodel.WorkspacePermissionWorkspaceView {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspacemodel.WorkspacePermissionWorkspaceView)
			}
			if permissionChecker.lastWorkspaceID != "workspace-1" {
				t.Fatalf("workspace checked = %q, want workspace-1", permissionChecker.lastWorkspaceID)
			}
			if service.totalCalls() != 0 {
				t.Fatalf("service calls = %d, want 0", service.totalCalls())
			}
			if actionRepo.calls != 0 {
				t.Fatalf("action repo calls = %d, want 0", actionRepo.calls)
			}
			if runRepo.calls != 0 {
				t.Fatalf("run repo calls = %d, want 0", runRepo.calls)
			}
		})
	}
}

func TestTaskMutationRoutesRequireWorkspaceManageBeforeMutation(t *testing.T) {
	taskBody := `{"workspace_id":"workspace-1","name":"task","schedule_type":"cron","timezone":"UTC","schedule_config":{"expression":"0 * * * *"},"actions":[{"action_type":"run_workflow","config":{}}]}`
	tests := []struct {
		name   string
		method string
		target string
		body   string
		params gin.Params
		call   func(*TaskHandler, *gin.Context)
	}{
		{
			name:   "generate draft",
			method: http.MethodPost,
			target: "/automations/tasks/draft/generate",
			body:   `{"workspace_id":"workspace-1","prompt":"send a report","model":"gpt-test"}`,
			call:   (*TaskHandler).GenerateTaskDraft,
		},
		{
			name:   "create task",
			method: http.MethodPost,
			target: "/automations/tasks",
			body:   taskBody,
			call:   (*TaskHandler).CreateTask,
		},
		{
			name:   "update task",
			method: http.MethodPatch,
			target: "/automations/tasks/task-1",
			body:   taskBody,
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).UpdateTask,
		},
		{
			name:   "run task now",
			method: http.MethodPost,
			target: "/automations/tasks/task-1/run?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).RunTaskNow,
		},
		{
			name:   "pause task",
			method: http.MethodPost,
			target: "/automations/tasks/task-1/pause?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).PauseTask,
		},
		{
			name:   "resume task",
			method: http.MethodPost,
			target: "/automations/tasks/task-1/resume?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).ResumeTask,
		},
		{
			name:   "archive task",
			method: http.MethodPost,
			target: "/automations/tasks/task-1/archive?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).ArchiveTask,
		},
		{
			name:   "delete task",
			method: http.MethodDelete,
			target: "/automations/tasks/task-1?workspace_id=workspace-1",
			params: gin.Params{{Key: "id", Value: "task-1"}},
			call:   (*TaskHandler).DeleteTask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &taskPermissionService{}
			actionRepo := &taskPermissionActionRepository{}
			runRepo := &taskPermissionRunRepository{}
			permissionChecker := &taskPermissionOrganizationService{allowed: false}
			handler := &TaskHandler{
				service:             service,
				actionRepo:          actionRepo,
				runRepo:             runRepo,
				organizationService: permissionChecker,
			}
			ctx, recorder := newTaskPermissionContext(tt.method, tt.target, tt.body, tt.params)

			tt.call(handler, ctx)

			requireTaskPermissionDenied(t, recorder)
			if permissionChecker.lastPermission != workspacemodel.WorkspacePermissionWorkspaceManage {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspacemodel.WorkspacePermissionWorkspaceManage)
			}
			if permissionChecker.lastWorkspaceID != "workspace-1" {
				t.Fatalf("workspace checked = %q, want workspace-1", permissionChecker.lastWorkspaceID)
			}
			if service.totalCalls() != 0 {
				t.Fatalf("service calls = %d, want 0", service.totalCalls())
			}
			if actionRepo.calls != 0 {
				t.Fatalf("action repo calls = %d, want 0", actionRepo.calls)
			}
			if runRepo.calls != 0 {
				t.Fatalf("run repo calls = %d, want 0", runRepo.calls)
			}
		})
	}
}

func newTaskPermissionContext(method, target, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if body == "" {
		body = "{}"
	}
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	ctx.Set("account_id", "account-1")
	util.SetOrganizationID(ctx, "org-1")
	return ctx, recorder
}

func requireTaskPermissionDenied(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v; body=%s", err, recorder.Body.String())
	}
	if body.Code != strconv.Itoa(response.ErrPermissionDenied.Code) {
		t.Fatalf("response code = %s, want %d; body=%s", body.Code, response.ErrPermissionDenied.Code, recorder.Body.String())
	}
}

type taskPermissionService struct {
	automationdefinition.Service

	createCalls  int
	updateCalls  int
	getCalls     int
	listCalls    int
	countCalls   int
	runNowCalls  int
	pauseCalls   int
	resumeCalls  int
	archiveCalls int
	deleteCalls  int
}

func (s *taskPermissionService) totalCalls() int {
	return s.createCalls +
		s.updateCalls +
		s.getCalls +
		s.listCalls +
		s.countCalls +
		s.runNowCalls +
		s.pauseCalls +
		s.resumeCalls +
		s.archiveCalls +
		s.deleteCalls
}

func (s *taskPermissionService) CreateTask(context.Context, automationdto.CreateTaskRequest) (*automationdto.CreateTaskResult, error) {
	s.createCalls++
	return &automationdto.CreateTaskResult{}, nil
}

func (s *taskPermissionService) UpdateTask(context.Context, automationdto.TaskScope, string, automationdto.UpdateTaskRequest) (*automationdto.CreateTaskResult, error) {
	s.updateCalls++
	return &automationdto.CreateTaskResult{}, nil
}

func (s *taskPermissionService) GetTask(context.Context, automationdto.TaskScope, string) (*automationmodel.AutomationTask, error) {
	s.getCalls++
	return &automationmodel.AutomationTask{ID: "task-1"}, nil
}

func (s *taskPermissionService) ListTasks(context.Context, automationdto.TaskFilter) ([]*automationmodel.AutomationTask, error) {
	s.listCalls++
	return []*automationmodel.AutomationTask{}, nil
}

func (s *taskPermissionService) CountTasks(context.Context, automationdto.TaskFilter) (int64, error) {
	s.countCalls++
	return 0, nil
}

func (s *taskPermissionService) RunTaskNow(context.Context, automationdto.TaskScope, string) (*automationmodel.AutomationTaskRun, error) {
	s.runNowCalls++
	return &automationmodel.AutomationTaskRun{}, nil
}

func (s *taskPermissionService) PauseTask(context.Context, automationdto.TaskScope, string, string) error {
	s.pauseCalls++
	return nil
}

func (s *taskPermissionService) ResumeTask(context.Context, automationdto.TaskScope, string, string) error {
	s.resumeCalls++
	return nil
}

func (s *taskPermissionService) ArchiveTask(context.Context, automationdto.TaskScope, string, string) error {
	s.archiveCalls++
	return nil
}

func (s *taskPermissionService) DeleteTask(context.Context, automationdto.TaskScope, string) error {
	s.deleteCalls++
	return nil
}

type taskPermissionActionRepository struct {
	automationrepo.ActionRepository
	calls int
}

func (r *taskPermissionActionRepository) ListByTaskID(context.Context, *gorm.DB, string) ([]*automationmodel.AutomationTaskAction, error) {
	r.calls++
	return []*automationmodel.AutomationTaskAction{}, nil
}

type taskPermissionRunRepository struct {
	automationrepo.RunRepository
	calls int
}

func (r *taskPermissionRunRepository) CountTaskRuns(context.Context, *gorm.DB, automationdto.TaskScope, string) (int64, error) {
	r.calls++
	return 0, nil
}

func (r *taskPermissionRunRepository) ListTaskRuns(context.Context, *gorm.DB, automationdto.TaskScope, string, int, int) ([]*automationmodel.AutomationTaskRun, error) {
	r.calls++
	return []*automationmodel.AutomationTaskRun{}, nil
}

func (r *taskPermissionRunRepository) ListActionRunsByTaskRunID(context.Context, *gorm.DB, string) ([]*automationmodel.AutomationActionRun, error) {
	r.calls++
	return []*automationmodel.AutomationActionRun{}, nil
}

type taskPermissionOrganizationService struct {
	interfaces.OrganizationService

	allowed         bool
	lastWorkspaceID string
	lastPermission  workspacemodel.WorkspacePermissionCode
}

func (s *taskPermissionOrganizationService) CheckWorkspacePermission(_ context.Context, _, workspaceID, _ string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.lastWorkspaceID = workspaceID
	s.lastPermission = permissionCode
	return s.allowed, nil
}

func TestGetTaskCountsUsesAuthorizedWorkspaceScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &taskCountsDefinitionService{
		counts: map[automationmodel.AutomationTaskStatus]int64{
			automationmodel.AutomationTaskStatusDraft:     1,
			automationmodel.AutomationTaskStatusActive:    2,
			automationmodel.AutomationTaskStatusPaused:    3,
			automationmodel.AutomationTaskStatusCompleted: 4,
			automationmodel.AutomationTaskStatusArchived:  5,
		},
	}
	organization := &taskCountsOrganizationService{allowed: true}
	handler := NewTaskHandler(
		service,
		nil,
		nil,
		nil,
		organization,
		&taskCountsWorkspaceService{workspace: &workspacemodel.Workspace{ID: "workspace-1", OrganizationID: stringPointer("org-1")}},
		nil,
		nil,
	)

	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	request := httptest.NewRequest(http.MethodGet, "/automations/tasks/counts?workspace_id=workspace-1", nil)
	context.Request = request
	util.SetOrganizationID(context, "org-1")
	context.Set("account_id", "account-1")

	handler.GetTaskCounts(context)

	if writer.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", writer.Code, http.StatusOK, writer.Body.String())
	}
	if organization.organizationID != "org-1" || organization.workspaceID != "workspace-1" || organization.accountID != "account-1" {
		t.Fatalf("permission scope = (%q, %q, %q), want (org-1, workspace-1, account-1)", organization.organizationID, organization.workspaceID, organization.accountID)
	}
	if organization.permission != workspacemodel.WorkspacePermissionWorkspaceView {
		t.Fatalf("permission = %q, want %q", organization.permission, workspacemodel.WorkspacePermissionWorkspaceView)
	}
	if service.scope != (automationdto.TaskScope{OrganizationID: "org-1", WorkspaceID: "workspace-1"}) {
		t.Fatalf("count scope = %#v, want org-1/workspace-1", service.scope)
	}

	var response struct {
		Code string `json:"code"`
		Data struct {
			All       int64 `json:"all"`
			Active    int64 `json:"active"`
			Paused    int64 `json:"paused"`
			Completed int64 `json:"completed"`
			Archived  int64 `json:"archived"`
		} `json:"data"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "0" {
		t.Fatalf("response code = %q, want 0", response.Code)
	}
	if response.Data.All != 15 || response.Data.Active != 2 || response.Data.Paused != 3 || response.Data.Completed != 4 || response.Data.Archived != 5 {
		t.Fatalf("counts = %#v, want all=15 active=2 paused=3 completed=4 archived=5", response.Data)
	}
}

func TestGetTaskCountsRejectsUnauthorizedWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &taskCountsDefinitionService{}
	handler := NewTaskHandler(
		service,
		nil,
		nil,
		nil,
		&taskCountsOrganizationService{},
		&taskCountsWorkspaceService{workspace: &workspacemodel.Workspace{ID: "workspace-1", OrganizationID: stringPointer("org-1")}},
		nil,
		nil,
	)

	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	context.Request = httptest.NewRequest(http.MethodGet, "/automations/tasks/counts?workspace_id=workspace-1", nil)
	util.SetOrganizationID(context, "org-1")
	context.Set("account_id", "account-1")

	handler.GetTaskCounts(context)

	if writer.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", writer.Code, http.StatusForbidden, writer.Body.String())
	}
	if service.called {
		t.Fatal("count service must not run without workspace view permission")
	}
}

func TestRegisterRoutesRegistersStaticTaskCountsRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewTaskHandler(nil, nil, nil, nil, nil, nil, nil, nil).RegisterRoutes(router.Group(""))

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/automations/tasks/counts" {
			return
		}
	}
	t.Fatal("GET /automations/tasks/counts route is not registered")
}

type taskCountsDefinitionService struct {
	automationdefinition.Service
	counts map[automationmodel.AutomationTaskStatus]int64
	scope  automationdto.TaskScope
	called bool
}

func (s *taskCountsDefinitionService) CountTasksByStatus(_ context.Context, scope automationdto.TaskScope) (map[automationmodel.AutomationTaskStatus]int64, error) {
	s.called = true
	s.scope = scope
	return s.counts, nil
}

type taskCountsOrganizationService struct {
	interfaces.OrganizationService
	allowed        bool
	organizationID string
	workspaceID    string
	accountID      string
	permission     workspacemodel.WorkspacePermissionCode
}

func (s *taskCountsOrganizationService) CheckWorkspacePermission(
	_ context.Context,
	organizationID, workspaceID, accountID string,
	permission workspacemodel.WorkspacePermissionCode,
) (bool, error) {
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permission = permission
	return s.allowed, nil
}

type taskCountsWorkspaceService struct {
	interfaces.WorkspaceManagementService
	workspace *workspacemodel.Workspace
}

func (s *taskCountsWorkspaceService) GetWorkspaceByID(context.Context, string) (*workspacemodel.Workspace, error) {
	return s.workspace, nil
}

func stringPointer(value string) *string {
	return &value
}
