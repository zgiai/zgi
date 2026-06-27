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
	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestPromptBuilderToolsRequireWorkspacePermissionBeforeBodyBinding(t *testing.T) {
	workspaceID := "workspace-1"
	tests := []struct {
		name            string
		method          func(*PromptHandler, *gin.Context)
		target          string
		wantPermissions []workspace_model.WorkspacePermissionCode
		serviceCalled   func(*promptPermissionService) bool
	}{
		{
			name:            "optimize",
			method:          (*PromptHandler).OptimizePrompt,
			target:          "/prompts/optimize",
			wantPermissions: []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionWorkspaceView},
			serviceCalled:   func(s *promptPermissionService) bool { return s.optimizeCalled },
		},
		{
			name:            "optimize stream",
			method:          (*PromptHandler).OptimizePromptStream,
			target:          "/prompts/optimize/stream",
			wantPermissions: []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionWorkspaceView},
			serviceCalled:   func(s *promptPermissionService) bool { return s.optimizeStreamCalled },
		},
		{
			name:            "playground stream",
			method:          (*PromptHandler).PlaygroundPromptStream,
			target:          "/prompts/playground/stream",
			wantPermissions: []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionWorkspaceView},
			serviceCalled:   func(s *promptPermissionService) bool { return s.playgroundCalled },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &promptPermissionService{}
			organizationService := &promptPermissionOrganizationService{allowed: false}
			handler := NewPromptHandler(service, nil, organizationService)
			ctx, recorder := newPromptPermissionContext(http.MethodPost, tt.target, `{"broken":`, workspaceID)

			tt.method(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			requirePromptResponseCode(t, recorder, response.ErrPermissionDenied)
			if !organizationService.checked {
				t.Fatalf("expected workspace permission check")
			}
			if organizationService.workspaceID != workspaceID {
				t.Fatalf("workspace checked = %q, want %q", organizationService.workspaceID, workspaceID)
			}
			if got, want := organizationService.permissionCodes, tt.wantPermissions; !samePromptPermissionCodes(got, want) {
				t.Fatalf("permission codes = %v, want %v", got, want)
			}
			if tt.serviceCalled(service) {
				t.Fatalf("prompt service should not be called after missing workspace permission")
			}
		})
	}
}

func TestPromptBuilderToolsRequireCurrentWorkspaceBeforeBodyBinding(t *testing.T) {
	service := &promptPermissionService{}
	organizationService := &promptPermissionOrganizationService{allowed: true}
	handler := NewPromptHandler(service, nil, organizationService)
	ctx, recorder := newPromptPermissionContext(http.MethodPost, "/prompts/optimize", `{"broken":`, "")

	handler.OptimizePrompt(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requirePromptResponseCode(t, recorder, response.ErrPermissionDenied)
	if organizationService.checked {
		t.Fatalf("workspace permission check should not run without current workspace")
	}
	if service.optimizeCalled {
		t.Fatalf("prompt service should not be called without current workspace")
	}
}

func TestPromptBuilderToolsResolveExplicitWorkspaceIDFromBodyBeforeBinding(t *testing.T) {
	service := &promptPermissionService{}
	organizationService := &promptPermissionOrganizationService{allowed: false}
	handler := NewPromptHandler(service, nil, organizationService)
	ctx, recorder := newPromptPermissionContext(http.MethodPost, "/prompts/optimize", `{"workspace_id":"workspace-body"}`, "")

	handler.OptimizePrompt(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requirePromptResponseCode(t, recorder, response.ErrPermissionDenied)
	if !organizationService.checked {
		t.Fatalf("expected workspace permission check")
	}
	if organizationService.workspaceID != "workspace-body" {
		t.Fatalf("workspace checked = %q, want workspace-body", organizationService.workspaceID)
	}
	if service.optimizeCalled {
		t.Fatalf("prompt service should not be called after missing workspace permission")
	}
}

func TestPromptBuilderToolsResolveCurrentWorkspaceFromAccountContextBeforeBinding(t *testing.T) {
	workspaceID := "workspace-from-account"
	service := &promptPermissionService{}
	organizationService := &promptPermissionOrganizationService{allowed: true}
	accountService := &promptPermissionAccountService{
		context: &authmodel.AccountContext{
			AccountID:          "account-1",
			CurrentWorkspaceID: &workspaceID,
		},
	}
	handler := NewPromptHandler(service, accountService, organizationService)
	ctx, recorder := newPromptPermissionContext(http.MethodPost, "/prompts/optimize", `{"broken":`, "")

	handler.OptimizePrompt(ctx)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("current workspace from account context should pass permission guard, body=%s", recorder.Body.String())
	}
	if !organizationService.checked {
		t.Fatalf("expected workspace permission check")
	}
	if organizationService.workspaceID != workspaceID {
		t.Fatalf("workspace checked = %q, want %q", organizationService.workspaceID, workspaceID)
	}
	if service.optimizeCalled {
		t.Fatalf("prompt service should not be called when body binding fails")
	}
	requirePromptResponseCode(t, recorder, response.ErrInvalidParam)
}

func newPromptPermissionContext(method, target, body, workspaceID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("account_id", "account-1")
	util.SetOrganizationID(ctx, "org-1")
	if workspaceID != "" {
		util.SetWorkspaceScopeCompat(ctx, workspaceID)
	}
	return ctx, recorder
}

func requirePromptResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, want response.ErrorCode) {
	t.Helper()
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v; body=%s", err, recorder.Body.String())
	}
	if body.Code != strconv.Itoa(want.Code) {
		t.Fatalf("response code = %s, want %d; body=%s", body.Code, want.Code, recorder.Body.String())
	}
}

func samePromptPermissionCodes(got, want []workspace_model.WorkspacePermissionCode) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type promptPermissionService struct {
	promptservice.PromptService

	optimizeCalled       bool
	optimizeStreamCalled bool
	playgroundCalled     bool
}

func (s *promptPermissionService) Optimize(context.Context, string, string, string, promptdto.PromptOptimizeRequest) (*promptdto.PromptOptimizeResponse, error) {
	s.optimizeCalled = true
	return nil, nil
}

func (s *promptPermissionService) OptimizeStream(context.Context, string, string, string, promptdto.PromptOptimizeRequest, func(promptservice.PromptOptimizeStreamEvent) error) (*promptdto.PromptOptimizeResponse, error) {
	s.optimizeStreamCalled = true
	return nil, nil
}

func (s *promptPermissionService) PlaygroundStream(context.Context, string, string, string, promptdto.PromptPlaygroundRequest, func(promptservice.PromptOptimizeStreamEvent) error) error {
	s.playgroundCalled = true
	return nil
}

type promptPermissionOrganizationService struct {
	interfaces.OrganizationService

	allowed         bool
	checked         bool
	workspaceID     string
	permissionCodes []workspace_model.WorkspacePermissionCode
}

func (s *promptPermissionOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, permissionCodes ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checked = true
	s.workspaceID = workspaceID
	s.permissionCodes = append([]workspace_model.WorkspacePermissionCode(nil), permissionCodes...)
	return s.allowed, nil
}

type promptPermissionAccountService struct {
	interfaces.AccountService

	context *authmodel.AccountContext
}

func (s *promptPermissionAccountService) GetAccountContext(context.Context, string) (*authmodel.AccountContext, error) {
	return s.context, nil
}
