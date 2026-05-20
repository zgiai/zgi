package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/dto"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
)

func TestAgentsService_UpdateWebAppStatus_AllowsManagerEditor(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	accountID := "99999999-9999-9999-9999-999999999999"
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:        agentID,
			TenantID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WebAppID:  uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true},
	}

	resp, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
		Reason: "maintenance",
	})
	require.NoError(t, err)
	require.Equal(t, agentID.String(), resp.AgentID)
	require.Equal(t, "33333333-3333-3333-3333-333333333333", resp.WebAppID)
	require.Equal(t, "inactive", resp.WebAppStatus)
	require.Equal(t, AgentWebAppStatusInactive, repo.lastStatus)
	require.Equal(t, "maintenance", repo.lastReason)
	require.Equal(t, accountID, repo.lastUpdatedBy)
}

func TestAgentsService_UpdateWebAppStatus_RejectsInvalidInputs(t *testing.T) {
	service := &agentsService{}

	_, err := service.UpdateWebAppStatus(webAppStatusTestContext(), "agent-1", dto.UpdateWebAppStatusRequest{
		Status: "archived",
	})
	require.ErrorIs(t, err, errInvalidWebAppStatus)

	_, err = service.UpdateWebAppStatus(webAppStatusTestContext(), "agent-1", dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
		Reason: strings.Repeat("a", maxWebAppOfflineReasonLength+1),
	})
	require.ErrorIs(t, err, errWebAppOfflineReasonTooLong)
}

func TestAgentsService_UpdateWebAppStatus_RejectsMissingPermission(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WebAppID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: false},
	}

	_, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
	})
	require.EqualError(t, err, "permission denied")
	require.False(t, repo.updateCalled)
}

func TestAgentsService_UpdateWebAppStatus_RejectsSystemManagedAgent(t *testing.T) {
	ctx := webAppStatusTestContext()
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:       agentID,
			TenantID: uuid.Nil,
			WebAppID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
	}
	service := &agentsService{
		agentsRepo:     repo,
		accountService: &stubWebAppStatusAccountService{isEditor: true},
	}

	_, err := service.UpdateWebAppStatus(ctx, agentID.String(), dto.UpdateWebAppStatusRequest{
		Status: string(AgentWebAppStatusInactive),
	})
	require.EqualError(t, err, "agent not found")
	require.False(t, repo.updateCalled)
}

func webAppStatusTestContext() context.Context {
	ctx := context.WithValue(context.Background(), "account_id", "99999999-9999-9999-9999-999999999999")
	return context.WithValue(ctx, "tenant_id", "88888888-8888-8888-8888-888888888888")
}

type stubWebAppStatusRepository struct {
	AgentsRepository

	agent         *Agent
	updateCalled  bool
	lastStatus    AgentWebAppStatus
	lastReason    string
	lastUpdatedBy string
}

func (s *stubWebAppStatusRepository) GetByID(_ context.Context, id string) (*Agent, error) {
	if s.agent == nil || s.agent.ID.String() != id {
		return nil, errors.New("agent not found")
	}
	return s.agent, nil
}

func (s *stubWebAppStatusRepository) UpdateWebAppStatus(_ context.Context, _ string, status AgentWebAppStatus, reason string, updatedBy string) error {
	s.updateCalled = true
	s.lastStatus = status
	s.lastReason = reason
	s.lastUpdatedBy = updatedBy
	s.agent.WebAppStatus = status
	s.agent.WebAppOfflineReason = reason
	s.agent.UpdatedAt = time.Now()
	return nil
}

type stubWebAppStatusAccountService struct {
	interfaces.AccountService

	isEditor bool
	err      error
}

func (s *stubWebAppStatusAccountService) IsEditor(_ context.Context, _ string) (bool, error) {
	return s.isEditor, s.err
}

type stubWebAppStatusOrganizationService struct {
	interfaces.OrganizationService

	allowed bool
	err     error
}

func (s *stubWebAppStatusOrganizationService) CheckWorkspacePermission(_ context.Context, _, _, _ string, _ workspace_model.WorkspacePermissionCode) (bool, error) {
	return s.allowed, s.err
}
