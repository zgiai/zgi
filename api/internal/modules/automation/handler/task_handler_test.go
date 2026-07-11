package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
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
