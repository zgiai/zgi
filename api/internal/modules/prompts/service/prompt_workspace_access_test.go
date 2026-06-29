package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

func TestPromptOptimizerRequiresCurrentWorkspaceBeforeModelSetup(t *testing.T) {
	svc := &promptService{
		organizationService: &promptWorkspaceAccessOrganizationService{
			workspaces: []*workspace_model.Workspace{{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal}},
			allowed:    true,
		},
	}

	_, err := svc.Optimize(context.Background(), "org-1", "account-1", "", promptdto.PromptOptimizeRequest{RawPrompt: "Improve this prompt"})

	if err == nil || !strings.Contains(err.Error(), "workspace is required") {
		t.Fatalf("Optimize() error = %v, want workspace required", err)
	}
}

func TestPromptOptimizerRequiresWorkspaceViewBeforeModelSetup(t *testing.T) {
	organizationService := &promptWorkspaceAccessOrganizationService{
		workspaces: []*workspace_model.Workspace{{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal}},
		allowed:    false,
	}
	svc := &promptService{organizationService: organizationService}

	_, err := svc.Optimize(context.Background(), "org-1", "account-1", "workspace-1", promptdto.PromptOptimizeRequest{RawPrompt: "Improve this prompt"})

	if err == nil || !strings.Contains(err.Error(), "workspace not accessible") {
		t.Fatalf("Optimize() error = %v, want workspace not accessible", err)
	}
	if !organizationService.permissionChecked {
		t.Fatalf("expected workspace permission check")
	}
	if got := organizationService.permissionCodes; len(got) != 1 || got[0] != workspace_model.WorkspacePermissionWorkspaceView {
		t.Fatalf("permission codes = %v, want [%s]", got, workspace_model.WorkspacePermissionWorkspaceView)
	}
}

func TestPromptPlaygroundRequiresWorkspaceViewBeforeModelSetup(t *testing.T) {
	organizationService := &promptWorkspaceAccessOrganizationService{
		workspaces: []*workspace_model.Workspace{{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal}},
		allowed:    false,
	}
	svc := &promptService{organizationService: organizationService}

	err := svc.PlaygroundStream(context.Background(), "org-1", "account-1", "workspace-1", promptdto.PromptPlaygroundRequest{Prompt: "Answer clearly"}, nil)

	if err == nil || !strings.Contains(err.Error(), "workspace not accessible") {
		t.Fatalf("PlaygroundStream() error = %v, want workspace not accessible", err)
	}
	want := []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionWorkspaceView,
	}
	if got := organizationService.permissionCodes; !sameWorkspaceAccessPermissions(got, want) {
		t.Fatalf("permission codes = %v, want %v", got, want)
	}
}

func TestPromptListRequiresWorkspaceViewBeforePromptQuery(t *testing.T) {
	organizationService := &promptWorkspaceAccessOrganizationService{
		workspaces: []*workspace_model.Workspace{{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal}},
		allowed:    false,
	}
	svc := &promptService{organizationService: organizationService}

	resp, err := svc.List(context.Background(), "org-1", "account-1", promptdto.PromptListRequest{})

	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if resp == nil || len(resp.Data) != 0 || resp.Total != 0 {
		t.Fatalf("List() response = %#v, want empty response", resp)
	}
	want := []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionWorkspaceView,
	}
	if got := organizationService.permissionCodes; !sameWorkspaceAccessPermissions(got, want) {
		t.Fatalf("permission codes = %v, want %v", got, want)
	}
}

func TestOfficialPromptDetailRequiresWorkspaceViewBeforeVersionLookup(t *testing.T) {
	repo := &promptWorkspaceAccessRepository{
		prompt: &promptmodel.Prompt{
			ID:     "official-prompt-1",
			Source: promptmodel.PromptSourceOfficial,
		},
	}
	organizationService := &promptWorkspaceAccessOrganizationService{
		workspaces: []*workspace_model.Workspace{{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal}},
		allowed:    false,
	}
	svc := &promptService{
		repo:                repo,
		organizationService: organizationService,
	}

	_, err := svc.GetDetail(context.Background(), "org-1", "account-1", "official-prompt-1")

	if err == nil || !strings.Contains(err.Error(), "prompt not found") {
		t.Fatalf("GetDetail() error = %v, want prompt not found", err)
	}
	want := []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionWorkspaceView,
	}
	if got := organizationService.permissionCodes; !sameWorkspaceAccessPermissions(got, want) {
		t.Fatalf("permission codes = %v, want %v", got, want)
	}
	if repo.findVersionsCalled {
		t.Fatal("official prompt versions should not be loaded without workspace access")
	}
}

func TestPromptOptimizerRejectsPromptFromAnotherWorkspace(t *testing.T) {
	workspaceID := "workspace-1"
	otherWorkspaceID := "workspace-2"
	promptID := "prompt-1"
	svc := &promptService{
		repo: &promptWorkspaceAccessRepository{
			prompt: &promptmodel.Prompt{
				ID:             promptID,
				OrganizationID: stringPtr("org-1"),
				WorkspaceID:    &otherWorkspaceID,
				Source:         promptmodel.PromptSourceWorkspace,
			},
		},
		organizationService: &promptWorkspaceAccessOrganizationService{
			allowed: true,
			workspaces: []*workspace_model.Workspace{
				{ID: workspaceID, Status: workspace_model.WorkspaceStatusNormal},
				{ID: otherWorkspaceID, Status: workspace_model.WorkspaceStatusNormal},
			},
		},
	}

	_, err := svc.Optimize(context.Background(), "org-1", "account-1", workspaceID, promptdto.PromptOptimizeRequest{
		RawPrompt: "Improve this prompt",
		PromptID:  promptID,
	})

	if err == nil || !strings.Contains(err.Error(), "prompt not found") {
		t.Fatalf("Optimize() error = %v, want prompt not found", err)
	}
}

func sameWorkspaceAccessPermissions(got, want []workspace_model.WorkspacePermissionCode) bool {
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

type promptWorkspaceAccessOrganizationService struct {
	interfaces.OrganizationService

	isAdmin           bool
	allowed           bool
	workspaces        []*workspace_model.Workspace
	permissionChecked bool
	permissionCodes   []workspace_model.WorkspacePermissionCode
}

func (s *promptWorkspaceAccessOrganizationService) GetOrganizationWorkspacesList(context.Context, string) ([]*workspace_model.Workspace, error) {
	return append([]*workspace_model.Workspace(nil), s.workspaces...), nil
}

func (s *promptWorkspaceAccessOrganizationService) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return s.isAdmin, nil
}

func (s *promptWorkspaceAccessOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _ string, _ string, permissionCodes ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.permissionChecked = true
	s.permissionCodes = append([]workspace_model.WorkspacePermissionCode(nil), permissionCodes...)
	return s.allowed, nil
}

type promptWorkspaceAccessRepository struct {
	prompt             *promptmodel.Prompt
	findVersionsCalled bool
}

func (r *promptWorkspaceAccessRepository) DB() *gorm.DB { return nil }

func (r *promptWorkspaceAccessRepository) FindByID(_ context.Context, id string) (*promptmodel.Prompt, error) {
	if r.prompt != nil && r.prompt.ID == id {
		return r.prompt, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *promptWorkspaceAccessRepository) List(context.Context, *gorm.DB, int, int) ([]*promptmodel.Prompt, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) FindLatestVersions(context.Context, []string) (map[string]*promptmodel.PromptVersion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) FindVersions(context.Context, string) ([]*promptmodel.PromptVersion, error) {
	r.findVersionsCalled = true
	return nil, fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) ListOptimizationRuns(context.Context, *gorm.DB, int, int) ([]*promptmodel.PromptOptimizationRun, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) FindOptimizationRunByID(context.Context, string) (*promptmodel.PromptOptimizationRun, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) Create(context.Context, *promptmodel.Prompt) error {
	return fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) CreateVersion(context.Context, *promptmodel.PromptVersion) error {
	return fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) CreateOptimizationRun(context.Context, *promptmodel.PromptOptimizationRun) error {
	return fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) Update(context.Context, *promptmodel.Prompt) error {
	return fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) UpdateOptimizationRun(context.Context, *promptmodel.PromptOptimizationRun) error {
	return fmt.Errorf("not implemented")
}

func (r *promptWorkspaceAccessRepository) UpdateVersionLabels(context.Context, string, []string) error {
	return fmt.Errorf("not implemented")
}
