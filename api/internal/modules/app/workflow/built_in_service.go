package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// BuiltInWorkflowService defines the interface for built-in workflow business logic operations
// Requirements: 3.1, 3.2, 3.3
type BuiltInWorkflowService interface {
	// GetAllBuiltInWorkflows retrieves all built-in workflows
	// Requirement 3.1, 3.4: Query all internal workflows
	GetAllBuiltInWorkflows(ctx context.Context, audience runtimeauth.RuntimeAudience) ([]dto.BuiltInWorkflowDTO, error)

	// GetBuiltInWorkflowByScenario retrieves a built-in workflow by scenario name
	// Requirement 3.1, 3.2: Query by scenario with validation
	GetBuiltInWorkflowByScenario(ctx context.Context, scenario string, audience runtimeauth.RuntimeAudience) (*dto.BuiltInWorkflowDTO, error)

	// GetBuiltInWorkflowByID retrieves a built-in workflow by agent ID
	// Requirement 3.1: Query by ID
	GetBuiltInWorkflowByID(ctx context.Context, id uuid.UUID, audience runtimeauth.RuntimeAudience) (*dto.BuiltInWorkflowDTO, error)

	GetBuiltInWorkflowRuntimeSurfaces(ctx context.Context, scenario, organizationID string) (*dto.BuiltInWorkflowRuntimeSurfaceAuthorizationResponse, error)
	UpdateBuiltInWorkflowRuntimeSurfaces(ctx context.Context, scenario, organizationID string, req dto.UpdateAgentRuntimeSurfacesRequest) (*dto.BuiltInWorkflowRuntimeSurfaceAuthorizationResponse, error)
}

// builtInWorkflowService implements BuiltInWorkflowService interface
type builtInWorkflowService struct {
	repo      BuiltInWorkflowRepository
	authStore *runtimeauth.Store
	db        *gorm.DB
}

// NewBuiltInWorkflowService creates a new BuiltInWorkflowService instance
func NewBuiltInWorkflowService(repo BuiltInWorkflowRepository, authStore *runtimeauth.Store, db *gorm.DB) BuiltInWorkflowService {
	return &builtInWorkflowService{
		repo:      repo,
		authStore: authStore,
		db:        db,
	}
}

// GetAllBuiltInWorkflows retrieves all built-in workflows
// Requirement 3.1, 3.4: Query all internal workflows
func (s *builtInWorkflowService) GetAllBuiltInWorkflows(ctx context.Context, audience runtimeauth.RuntimeAudience) ([]dto.BuiltInWorkflowDTO, error) {
	logger.Info("Getting all built-in workflows")

	// Validate repository
	if s.repo == nil {
		logger.Error("Built-in workflow repository not initialized", nil)
		return nil, fmt.Errorf("built-in workflow repository not initialized")
	}

	// Query repository
	workflows, err := s.repo.GetAllBuiltInWorkflows(ctx)
	if err != nil {
		logger.Error("Failed to get all built-in workflows", err)
		return nil, fmt.Errorf("failed to retrieve built-in workflows: %w", err)
	}

	filtered, err := s.filterAuthorizedBuiltInWorkflows(ctx, workflows, audience)
	if err != nil {
		logger.Error("Failed to filter built-in workflows by runtime authorization", err)
		return nil, fmt.Errorf("failed to authorize built-in workflows: %w", err)
	}

	logger.Info("Successfully retrieved built-in workflows", "count", len(filtered))
	return filtered, nil
}

// GetBuiltInWorkflowByScenario retrieves a built-in workflow by scenario name
// Requirement 3.1, 3.2, 3.3: Query by scenario with validation and error handling
func (s *builtInWorkflowService) GetBuiltInWorkflowByScenario(ctx context.Context, scenario string, audience runtimeauth.RuntimeAudience) (*dto.BuiltInWorkflowDTO, error) {
	logger.Info("Getting built-in workflow by scenario", "scenario", scenario)

	// Validate repository
	if s.repo == nil {
		logger.Error("Built-in workflow repository not initialized", fmt.Errorf("repository is nil"))
		return nil, fmt.Errorf("built-in workflow repository not initialized")
	}

	// Validate scenario name
	// Requirement 3.3: Business logic for scenario validation
	if err := validateScenarioName(scenario); err != nil {
		logger.Error("Invalid scenario name", err)
		return nil, fmt.Errorf("invalid scenario name: %w", err)
	}

	// Query repository
	workflow, err := s.repo.GetBuiltInWorkflowByScenario(ctx, scenario)
	if err != nil {
		// Requirement 3.2, 3.3: Error handling for not found cases
		if strings.Contains(err.Error(), "not found") {
			logger.Warn("Built-in workflow not found", "scenario", scenario)
			return nil, fmt.Errorf("built-in workflow not found for scenario '%s'", scenario)
		}
		logger.Error("Failed to get built-in workflow by scenario", err)
		return nil, fmt.Errorf("failed to retrieve built-in workflow: %w", err)
	}

	allowed, err := s.allowsBuiltInWorkflow(ctx, workflow.AgentID, audience)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("built-in workflow is not enabled for current audience")
	}

	logger.Info("Successfully retrieved built-in workflow", "scenario", scenario, "agentID", workflow.AgentID)
	return workflow, nil
}

// GetBuiltInWorkflowByID retrieves a built-in workflow by agent ID
// Requirement 3.1: Query by ID
func (s *builtInWorkflowService) GetBuiltInWorkflowByID(ctx context.Context, id uuid.UUID, audience runtimeauth.RuntimeAudience) (*dto.BuiltInWorkflowDTO, error) {
	logger.Info("Getting built-in workflow by ID", "id", id)

	// Validate repository
	if s.repo == nil {
		logger.Error("Built-in workflow repository not initialized", fmt.Errorf("repository is nil"))
		return nil, fmt.Errorf("built-in workflow repository not initialized")
	}

	// Validate UUID
	if id == uuid.Nil {
		logger.Error("Invalid UUID provided", fmt.Errorf("UUID is nil"))
		return nil, fmt.Errorf("invalid UUID: cannot be nil")
	}

	// Query repository
	workflow, err := s.repo.GetBuiltInWorkflowByID(ctx, id)
	if err != nil {
		// Requirement 3.2, 3.3: Error handling for not found cases
		if strings.Contains(err.Error(), "not found") {
			logger.Warn("Built-in workflow not found", "id", id)
			return nil, fmt.Errorf("built-in workflow not found for ID '%s'", id)
		}
		logger.Error("Failed to get built-in workflow by ID", err)
		return nil, fmt.Errorf("failed to retrieve built-in workflow: %w", err)
	}

	allowed, err := s.allowsBuiltInWorkflow(ctx, workflow.AgentID, audience)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("built-in workflow is not enabled for current audience")
	}

	logger.Info("Successfully retrieved built-in workflow", "id", id, "scenario", workflow.Scenario)
	return workflow, nil
}

func (s *builtInWorkflowService) GetBuiltInWorkflowRuntimeSurfaces(ctx context.Context, scenario, organizationID string) (*dto.BuiltInWorkflowRuntimeSurfaceAuthorizationResponse, error) {
	organizationUUID, err := parseBuiltInRuntimeOrganizationID(organizationID)
	if err != nil {
		return nil, err
	}
	workflow, err := s.builtInWorkflowByScenario(ctx, scenario)
	if err != nil {
		return nil, err
	}

	authStore := s.authStore
	if authStore == nil {
		authStore = runtimeauth.NewStore(nil)
	}
	auth, err := authStore.GetResourceAuthorization(ctx, runtimeauth.PublishedRuntimeResourceBuiltinWorkflow, workflow.AgentID, defaultBuiltInWorkflowPolicy())
	if err != nil {
		return nil, err
	}

	responseOrganizationID := organizationUUID.String()
	if auth.OrganizationID != uuid.Nil {
		responseOrganizationID = auth.OrganizationID.String()
	}

	return &dto.BuiltInWorkflowRuntimeSurfaceAuthorizationResponse{
		Scenario:       workflow.Scenario,
		AgentID:        workflow.AgentID.String(),
		OrganizationID: responseOrganizationID,
		Surfaces:       builtInRuntimeSurfaceAuthorizationDTOs(auth.Surfaces),
	}, nil
}

func (s *builtInWorkflowService) UpdateBuiltInWorkflowRuntimeSurfaces(ctx context.Context, scenario, organizationID string, req dto.UpdateAgentRuntimeSurfacesRequest) (*dto.BuiltInWorkflowRuntimeSurfaceAuthorizationResponse, error) {
	organizationUUID, err := parseBuiltInRuntimeOrganizationID(organizationID)
	if err != nil {
		return nil, err
	}
	if s.authStore == nil {
		return nil, fmt.Errorf("published runtime authorization store is not initialized")
	}
	workflow, err := s.builtInWorkflowByScenario(ctx, scenario)
	if err != nil {
		return nil, err
	}
	auth, err := builtInRuntimeAuthorizationFromUpdateRequest(workflow.AgentID, organizationUUID, req)
	if err != nil {
		return nil, err
	}
	if err := s.validateBuiltInRuntimeGrantSubjects(ctx, organizationUUID, auth.Surfaces); err != nil {
		return nil, err
	}
	if err := s.authStore.SaveResourceAuthorization(ctx, auth); err != nil {
		return nil, err
	}
	return s.GetBuiltInWorkflowRuntimeSurfaces(ctx, workflow.Scenario, organizationUUID.String())
}

func (s *builtInWorkflowService) filterAuthorizedBuiltInWorkflows(ctx context.Context, workflows []dto.BuiltInWorkflowDTO, audience runtimeauth.RuntimeAudience) ([]dto.BuiltInWorkflowDTO, error) {
	if len(workflows) == 0 {
		return workflows, nil
	}
	if s.authStore == nil {
		return workflows, nil
	}

	if len(audience.DepartmentIDs) == 0 {
		departmentIDs, err := s.departmentIDsForAudience(ctx, audience)
		if err != nil {
			return nil, err
		}
		audience.DepartmentIDs = departmentIDs
	}
	if len(audience.WorkspaceIDs) == 0 {
		workspaceIDs, err := s.workspaceIDsForAudience(ctx, audience)
		if err != nil {
			return nil, err
		}
		audience.WorkspaceIDs = workspaceIDs
	}

	candidates := make([]runtimeauth.ResourceAuthorizationCandidate, 0, len(workflows))
	for _, workflow := range workflows {
		candidates = append(candidates, runtimeauth.ResourceAuthorizationCandidate{
			ResourceID: workflow.AgentID,
			Fallback:   defaultBuiltInWorkflowPolicy(),
		})
	}

	allowedIDs, err := s.authStore.FilterAuthorizedResourceIDs(ctx, runtimeauth.PublishedRuntimeResourceBuiltinWorkflow, runtimeauth.PublishedRuntimeSurfaceBuiltinApp, audience.OrganizationID, candidates, audience)
	if err != nil {
		return nil, err
	}
	allowedIDSet := make(map[uuid.UUID]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		allowedIDSet[id] = struct{}{}
	}

	out := make([]dto.BuiltInWorkflowDTO, 0, len(workflows))
	for _, workflow := range workflows {
		if _, ok := allowedIDSet[workflow.AgentID]; ok {
			out = append(out, workflow)
		}
	}
	return out, nil
}

func (s *builtInWorkflowService) builtInWorkflowByScenario(ctx context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error) {
	if s.repo == nil {
		logger.Error("Built-in workflow repository not initialized", fmt.Errorf("repository is nil"))
		return nil, fmt.Errorf("built-in workflow repository not initialized")
	}
	if err := validateScenarioName(scenario); err != nil {
		return nil, fmt.Errorf("invalid scenario name: %w", err)
	}
	workflow, err := s.repo.GetBuiltInWorkflowByScenario(ctx, scenario)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("built-in workflow not found for scenario '%s'", scenario)
		}
		return nil, fmt.Errorf("failed to retrieve built-in workflow: %w", err)
	}
	return workflow, nil
}

func (s *builtInWorkflowService) allowsBuiltInWorkflow(ctx context.Context, agentID uuid.UUID, audience runtimeauth.RuntimeAudience) (bool, error) {
	if s.authStore == nil {
		return true, nil
	}
	auth, err := s.authStore.GetResourceAuthorization(ctx, runtimeauth.PublishedRuntimeResourceBuiltinWorkflow, agentID, defaultBuiltInWorkflowPolicy())
	if err != nil {
		return false, err
	}
	if auth.HasGrantType(runtimeauth.PublishedRuntimeSurfaceBuiltinApp, runtimeauth.PublishedRuntimeSubjectDepartment) && len(audience.DepartmentIDs) == 0 {
		departmentIDs, err := s.departmentIDsForAudience(ctx, audience)
		if err != nil {
			return false, err
		}
		audience.DepartmentIDs = departmentIDs
	}
	if auth.HasGrantType(runtimeauth.PublishedRuntimeSurfaceBuiltinApp, runtimeauth.PublishedRuntimeSubjectWorkspace) && len(audience.WorkspaceIDs) == 0 {
		workspaceIDs, err := s.workspaceIDsForAudience(ctx, audience)
		if err != nil {
			return false, err
		}
		audience.WorkspaceIDs = workspaceIDs
	}
	return auth.Allows(runtimeauth.PublishedRuntimeSurfaceBuiltinApp, audience), nil
}

func defaultBuiltInWorkflowPolicy() runtimeauth.PublishedRuntimePolicy {
	return runtimeauth.PublishedRuntimePolicy{
		BuiltinAppEnabled:  true,
		InternalInvocation: true,
	}
}

func parseBuiltInRuntimeOrganizationID(organizationID string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid organization id: %w", err)
	}
	return parsed, nil
}

func builtInRuntimeAuthorizationFromUpdateRequest(agentID, organizationID uuid.UUID, req dto.UpdateAgentRuntimeSurfacesRequest) (runtimeauth.ResourceAuthorization, error) {
	if len(req.Surfaces) == 0 {
		return runtimeauth.ResourceAuthorization{}, fmt.Errorf("at least one runtime surface is required")
	}

	seen := make(map[runtimeauth.PublishedRuntimeSurface]struct{}, len(req.Surfaces))
	surfaces := make([]runtimeauth.SurfaceAuthorization, 0, len(req.Surfaces))
	for _, item := range req.Surfaces {
		surface := runtimeauth.NormalizeSurface(item.Surface)
		if surface != runtimeauth.PublishedRuntimeSurfaceBuiltinApp && surface != runtimeauth.PublishedRuntimeSurfaceInternal {
			return runtimeauth.ResourceAuthorization{}, fmt.Errorf("built-in workflow runtime surface must be builtin_app or internal")
		}
		if _, exists := seen[surface]; exists {
			return runtimeauth.ResourceAuthorization{}, fmt.Errorf("duplicate runtime surface")
		}
		seen[surface] = struct{}{}

		if surface == runtimeauth.PublishedRuntimeSurfaceInternal && !item.Enabled {
			return runtimeauth.ResourceAuthorization{}, fmt.Errorf("internal runtime surface cannot be disabled")
		}

		grants, err := builtInRuntimeSurfaceGrantsFromRequest(surface, organizationID, item.Enabled, item.Grants)
		if err != nil {
			return runtimeauth.ResourceAuthorization{}, err
		}
		surfaces = append(surfaces, runtimeauth.SurfaceAuthorization{
			Surface:             surface,
			Enabled:             item.Enabled,
			CompatibilitySource: runtimeauth.PublishedRuntimeSourceGrant,
			Grants:              grants,
		})
	}

	return runtimeauth.ResourceAuthorization{
		ResourceType:   runtimeauth.PublishedRuntimeResourceBuiltinWorkflow,
		ResourceID:     agentID,
		OrganizationID: organizationID,
		Surfaces:       surfaces,
	}, nil
}

func (s *builtInWorkflowService) validateBuiltInRuntimeGrantSubjects(ctx context.Context, organizationID uuid.UUID, surfaces []runtimeauth.SurfaceAuthorization) error {
	if s.db == nil {
		return fmt.Errorf("database is required")
	}
	for _, surface := range surfaces {
		for _, grant := range surface.Grants {
			if grant.SubjectID == nil {
				continue
			}
			switch grant.SubjectType {
			case runtimeauth.PublishedRuntimeSubjectOrganization:
				if *grant.SubjectID != organizationID {
					return fmt.Errorf("runtime grant organization is not current organization")
				}
			case runtimeauth.PublishedRuntimeSubjectAccount:
				ok, err := s.builtInRuntimeGrantAccountInOrganization(ctx, organizationID, *grant.SubjectID)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("runtime grant account is not in organization")
				}
			case runtimeauth.PublishedRuntimeSubjectDepartment:
				ok, err := s.builtInRuntimeGrantDepartmentInOrganization(ctx, organizationID, *grant.SubjectID)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("runtime grant department is not in organization")
				}
			case runtimeauth.PublishedRuntimeSubjectWorkspace:
				ok, err := s.builtInRuntimeGrantWorkspaceInOrganization(ctx, organizationID, *grant.SubjectID)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("runtime grant workspace is not in organization")
				}
			}
		}
	}
	return nil
}

func (s *builtInWorkflowService) builtInRuntimeGrantAccountInOrganization(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID.String(), accountID.String(), workspacemodel.OrganizationMemberStatusActive).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant account: %w", err)
	}
	return count > 0, nil
}

func (s *builtInWorkflowService) builtInRuntimeGrantDepartmentInOrganization(ctx context.Context, organizationID, departmentID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.Department{}).
		Where("group_id = ? AND id = ? AND status = ?", organizationID.String(), departmentID.String(), workspacemodel.DepartmentStatusActive).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant department: %w", err)
	}
	return count > 0, nil
}

func (s *builtInWorkflowService) builtInRuntimeGrantWorkspaceInOrganization(ctx context.Context, organizationID, workspaceID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&workspacemodel.Workspace{}).
		Where("organization_id = ? AND id = ? AND status = ?", organizationID.String(), workspaceID.String(), workspacemodel.WorkspaceStatusNormal).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to validate runtime grant workspace: %w", err)
	}
	return count > 0, nil
}

func builtInRuntimeSurfaceGrantsFromRequest(surface runtimeauth.PublishedRuntimeSurface, organizationID uuid.UUID, surfaceEnabled bool, grants []dto.UpdateAgentRuntimeSurfaceGrant) ([]runtimeauth.SurfaceGrant, error) {
	switch surface {
	case runtimeauth.PublishedRuntimeSurfaceInternal:
		if len(grants) > 0 {
			for _, grant := range grants {
				if runtimeauth.NormalizeSubjectType(grant.SubjectType) != runtimeauth.PublishedRuntimeSubjectInternal {
					return nil, fmt.Errorf("internal runtime grants must use internal subject")
				}
				if grant.Enabled != nil && !*grant.Enabled {
					return nil, fmt.Errorf("internal runtime surface cannot be disabled")
				}
			}
		}
		return []runtimeauth.SurfaceGrant{{
			SubjectType: runtimeauth.PublishedRuntimeSubjectInternal,
			Enabled:     true,
		}}, nil
	case runtimeauth.PublishedRuntimeSurfaceBuiltinApp:
		if !surfaceEnabled {
			return nil, nil
		}
		if len(grants) == 0 {
			return nil, fmt.Errorf("builtin app surface requires at least one grant")
		}
	}

	out := make([]runtimeauth.SurfaceGrant, 0, len(grants))
	for _, item := range grants {
		subjectType := runtimeauth.NormalizeSubjectType(item.SubjectType)
		switch subjectType {
		case runtimeauth.PublishedRuntimeSubjectOrganization,
			runtimeauth.PublishedRuntimeSubjectAccount,
			runtimeauth.PublishedRuntimeSubjectDepartment,
			runtimeauth.PublishedRuntimeSubjectWorkspace:
		default:
			return nil, fmt.Errorf("builtin app grants must target organization, account, department, or workspace")
		}

		subjectID, err := builtInRuntimeGrantSubjectID(subjectType, item.SubjectID, organizationID)
		if err != nil {
			return nil, err
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		out = append(out, runtimeauth.SurfaceGrant{
			SubjectType: subjectType,
			SubjectID:   subjectID,
			Enabled:     enabled,
		})
	}
	return out, nil
}

func builtInRuntimeGrantSubjectID(subjectType runtimeauth.PublishedRuntimeSubjectType, rawID *string, organizationID uuid.UUID) (*uuid.UUID, error) {
	switch subjectType {
	case runtimeauth.PublishedRuntimeSubjectOrganization:
		if rawID == nil || strings.TrimSpace(*rawID) == "" {
			return copyBuiltInRuntimeUUIDPtr(organizationID), nil
		}
	case runtimeauth.PublishedRuntimeSubjectAccount, runtimeauth.PublishedRuntimeSubjectDepartment, runtimeauth.PublishedRuntimeSubjectWorkspace:
		if rawID == nil || strings.TrimSpace(*rawID) == "" {
			return nil, fmt.Errorf("runtime grant subject id is required")
		}
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*rawID))
	if err != nil {
		return nil, fmt.Errorf("invalid runtime grant subject id: %w", err)
	}
	if subjectType == runtimeauth.PublishedRuntimeSubjectOrganization && parsed != organizationID {
		return nil, fmt.Errorf("runtime grant organization is not current organization")
	}
	return &parsed, nil
}

func copyBuiltInRuntimeUUIDPtr(value uuid.UUID) *uuid.UUID {
	copied := value
	return &copied
}

func builtInRuntimeSurfaceAuthorizationDTOs(surfaces []runtimeauth.SurfaceAuthorization) []dto.AgentRuntimeSurfaceAuthorization {
	out := make([]dto.AgentRuntimeSurfaceAuthorization, 0, len(surfaces))
	for _, surface := range surfaces {
		if surface.Surface != runtimeauth.PublishedRuntimeSurfaceBuiltinApp && surface.Surface != runtimeauth.PublishedRuntimeSurfaceInternal {
			continue
		}
		out = append(out, dto.AgentRuntimeSurfaceAuthorization{
			Surface:             string(surface.Surface),
			Enabled:             surface.Enabled,
			CompatibilitySource: surface.CompatibilitySource,
			Grants:              builtInRuntimeSurfaceGrantDTOs(surface.Grants),
		})
	}
	return out
}

func builtInRuntimeSurfaceGrantDTOs(grants []runtimeauth.SurfaceGrant) []dto.AgentRuntimeSurfaceGrant {
	out := make([]dto.AgentRuntimeSurfaceGrant, 0, len(grants))
	for _, grant := range grants {
		var subjectID *string
		if grant.SubjectID != nil {
			value := grant.SubjectID.String()
			subjectID = &value
		}
		out = append(out, dto.AgentRuntimeSurfaceGrant{
			SubjectType: string(grant.SubjectType),
			SubjectID:   subjectID,
			Enabled:     grant.Enabled,
		})
	}
	return out
}

func (s *builtInWorkflowService) departmentIDsForAudience(ctx context.Context, audience runtimeauth.RuntimeAudience) ([]uuid.UUID, error) {
	if s.db == nil || audience.OrganizationID == uuid.Nil || audience.AccountID == uuid.Nil {
		return nil, nil
	}

	var rawDepartmentIDs []string
	if err := s.db.WithContext(ctx).
		Table("department_members").
		Select("department_members.department_id").
		Joins("JOIN departments ON departments.id = department_members.department_id").
		Where("department_members.account_id = ? AND departments.group_id = ? AND departments.status = ?", audience.AccountID, audience.OrganizationID, workspacemodel.DepartmentStatusActive).
		Scan(&rawDepartmentIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load audience departments: %w", err)
	}

	departmentIDs := make([]uuid.UUID, 0, len(rawDepartmentIDs))
	for _, rawDepartmentID := range rawDepartmentIDs {
		departmentID, err := uuid.Parse(strings.TrimSpace(rawDepartmentID))
		if err != nil {
			return nil, fmt.Errorf("invalid audience department id: %w", err)
		}
		departmentIDs = append(departmentIDs, departmentID)
	}
	return departmentIDs, nil
}

func (s *builtInWorkflowService) workspaceIDsForAudience(ctx context.Context, audience runtimeauth.RuntimeAudience) ([]uuid.UUID, error) {
	if s.db == nil || audience.OrganizationID == uuid.Nil || audience.AccountID == uuid.Nil {
		return nil, nil
	}

	var rawWorkspaceIDs []string
	if err := s.db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_members.workspace_id").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.account_id = ? AND workspaces.organization_id = ? AND workspaces.status = ?", audience.AccountID, audience.OrganizationID, workspacemodel.WorkspaceStatusNormal).
		Scan(&rawWorkspaceIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to load audience workspaces: %w", err)
	}

	workspaceIDs := make([]uuid.UUID, 0, len(rawWorkspaceIDs))
	for _, rawWorkspaceID := range rawWorkspaceIDs {
		workspaceID, err := uuid.Parse(strings.TrimSpace(rawWorkspaceID))
		if err != nil {
			return nil, fmt.Errorf("invalid audience workspace id: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	return workspaceIDs, nil
}

// validateScenarioName validates the scenario name format
// Requirement 3.3: Business logic for scenario validation
// Scenario names should be alphanumeric with underscores and hyphens only
func validateScenarioName(scenario string) error {
	// Check if empty
	if scenario == "" {
		return fmt.Errorf("scenario name cannot be empty")
	}

	// Check length (reasonable limits)
	if len(scenario) > 100 {
		return fmt.Errorf("scenario name too long (max 100 characters)")
	}

	// Check for valid characters (alphanumeric, underscore, hyphen)
	for _, char := range scenario {
		if !isValidScenarioChar(char) {
			return fmt.Errorf("scenario name contains invalid character '%c' (only alphanumeric, underscore, and hyphen allowed)", char)
		}
	}

	return nil
}

// isValidScenarioChar checks if a character is valid for scenario names
func isValidScenarioChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '_' ||
		char == '-'
}
