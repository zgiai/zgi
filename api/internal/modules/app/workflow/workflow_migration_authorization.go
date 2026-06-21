package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
)

var (
	errWebAppMigrationInvalidRequest = errors.New("webapp migration invalid request")
	errWebAppMigrationNotFound       = errors.New("webapp migration app not found")
	errWebAppMigrationAccessDenied   = errors.New("webapp migration access denied")
	errWebAppMigrationOffline        = errors.New("webapp migration offline")
)

type WebAppMigrationAuthorizer interface {
	AuthorizeWebAppMigration(ctx context.Context, webAppID, accountID string) error
}

type runtimeWebAppMigrationAuthorizer struct {
	agentsRepo agents.AgentsRepository
	store      *runtimeauth.Store
	db         *gorm.DB
}

func NewWebAppMigrationAuthorizer(agentsRepo agents.AgentsRepository, db *gorm.DB) WebAppMigrationAuthorizer {
	return &runtimeWebAppMigrationAuthorizer{
		agentsRepo: agentsRepo,
		store:      runtimeauth.NewStore(db),
		db:         db,
	}
}

func (h *WorkflowHandler) SetWebAppMigrationAuthorizer(authorizer WebAppMigrationAuthorizer) {
	h.webAppMigrationAuthorizer = authorizer
}

func (h *WorkflowHandler) getWebAppMigrationAuthorizer() WebAppMigrationAuthorizer {
	if h == nil {
		return nil
	}
	if h.webAppMigrationAuthorizer != nil {
		return h.webAppMigrationAuthorizer
	}

	ws, ok := h.workflowService.(*WorkflowService)
	if !ok || ws == nil || ws.agentsRepo == nil {
		return nil
	}
	return NewWebAppMigrationAuthorizer(ws.agentsRepo, database.GetDB())
}

func (a *runtimeWebAppMigrationAuthorizer) AuthorizeWebAppMigration(ctx context.Context, webAppID, accountID string) error {
	webAppID = strings.TrimSpace(webAppID)
	accountID = strings.TrimSpace(accountID)
	if webAppID == "" || accountID == "" {
		return errWebAppMigrationInvalidRequest
	}

	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		return errWebAppMigrationInvalidRequest
	}
	if a == nil || a.agentsRepo == nil {
		return fmt.Errorf("webapp migration authorizer is unavailable")
	}

	agent, err := a.agentsRepo.GetByWebAppID(ctx, webAppID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errWebAppMigrationNotFound
		}
		return fmt.Errorf("failed to load webapp migration agent: %w", err)
	}

	fallback := runtimeauth.PolicyFromAgentFields(string(agent.WebAppStatus), agent.EnableAPI)
	store := a.store
	if store == nil {
		store = runtimeauth.NewStore(a.db)
	}
	auth, err := store.GetResourceAuthorization(ctx, runtimeauth.PublishedRuntimeResourceAgent, agent.ID, fallback)
	if err != nil {
		return fmt.Errorf("failed to load webapp migration authorization: %w", err)
	}

	surface, ok := auth.Surface(runtimeauth.PublishedRuntimeSurfaceWebApp)
	if !ok {
		return errWebAppMigrationOffline
	}

	decision := surface.Evaluate(runtimeauth.RuntimeAudience{})
	if decision.Allowed {
		return nil
	}
	if decision.Reason == runtimeauth.RuntimeAccessDeniedDisabledSurface ||
		decision.Reason == runtimeauth.RuntimeAccessDeniedMissingSurface {
		return errWebAppMigrationOffline
	}

	audience, err := a.webAppMigrationAudienceForAccount(ctx, auth.OrganizationID, accountUUID)
	if err != nil {
		return err
	}
	decision = surface.Evaluate(audience)
	if decision.Allowed {
		return nil
	}
	if decision.Reason == runtimeauth.RuntimeAccessDeniedDisabledSurface ||
		decision.Reason == runtimeauth.RuntimeAccessDeniedMissingSurface {
		return errWebAppMigrationOffline
	}
	return errWebAppMigrationAccessDenied
}

func (a *runtimeWebAppMigrationAuthorizer) webAppMigrationAudienceForAccount(ctx context.Context, organizationID, accountID uuid.UUID) (runtimeauth.RuntimeAudience, error) {
	if a == nil || a.db == nil || organizationID == uuid.Nil || accountID == uuid.Nil {
		return runtimeauth.RuntimeAudience{}, nil
	}

	var memberCount int64
	if err := a.db.WithContext(ctx).
		Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID.String(), accountID.String(), workspacemodel.OrganizationMemberStatusActive).
		Count(&memberCount).Error; err != nil {
		return runtimeauth.RuntimeAudience{}, fmt.Errorf("failed to validate webapp migration organization member: %w", err)
	}
	if memberCount == 0 {
		return runtimeauth.RuntimeAudience{}, nil
	}

	departmentIDs, err := a.webAppMigrationDepartmentIDsForAccount(ctx, organizationID, accountID)
	if err != nil {
		return runtimeauth.RuntimeAudience{}, err
	}
	return runtimeauth.RuntimeAudience{
		OrganizationID: organizationID,
		AccountID:      accountID,
		DepartmentIDs:  departmentIDs,
	}, nil
}

func (a *runtimeWebAppMigrationAuthorizer) webAppMigrationDepartmentIDsForAccount(ctx context.Context, organizationID, accountID uuid.UUID) ([]uuid.UUID, error) {
	var rawDepartmentIDs []string
	if err := a.db.WithContext(ctx).
		Table("department_members").
		Select("department_members.department_id").
		Joins("JOIN departments ON departments.id = department_members.department_id").
		Where("department_members.account_id = ? AND departments.group_id = ? AND departments.status = ?", accountID, organizationID, workspacemodel.DepartmentStatusActive).
		Scan(&rawDepartmentIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load webapp migration audience departments: %w", err)
	}

	departmentIDs := make([]uuid.UUID, 0, len(rawDepartmentIDs))
	for _, rawDepartmentID := range rawDepartmentIDs {
		departmentID, err := uuid.Parse(strings.TrimSpace(rawDepartmentID))
		if err != nil {
			return nil, fmt.Errorf("invalid webapp migration audience department id: %w", err)
		}
		departmentIDs = append(departmentIDs, departmentID)
	}
	return departmentIDs, nil
}
