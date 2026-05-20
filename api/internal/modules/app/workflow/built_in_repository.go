package workflow

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
	"gorm.io/gorm"
)

// BuiltInWorkflowRepository defines the interface for built-in workflow data access operations
type BuiltInWorkflowRepository interface {
	GetAllBuiltInWorkflows(ctx context.Context) ([]dto.BuiltInWorkflowDTO, error)
	GetBuiltInWorkflowByID(ctx context.Context, id uuid.UUID) (*dto.BuiltInWorkflowDTO, error)
	GetBuiltInWorkflowByScenario(ctx context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error)
}

type builtInWorkflowRepository struct {
	db *gorm.DB
}

func NewBuiltInWorkflowRepository(db *gorm.DB) BuiltInWorkflowRepository {
	return &builtInWorkflowRepository{db: db}
}

func (r *builtInWorkflowRepository) GetAllBuiltInWorkflows(ctx context.Context) ([]dto.BuiltInWorkflowDTO, error) {
	return r.getAllLegacyBuiltInWorkflows(ctx)
}

func (r *builtInWorkflowRepository) GetBuiltInWorkflowByID(ctx context.Context, id uuid.UUID) (*dto.BuiltInWorkflowDTO, error) {
	return r.getLegacyBuiltInWorkflowByID(ctx, id)
}

func (r *builtInWorkflowRepository) GetBuiltInWorkflowByScenario(ctx context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error) {
	return r.getLegacyBuiltInWorkflowByScenario(ctx, scenario)
}

func (r *builtInWorkflowRepository) getAllLegacyBuiltInWorkflows(ctx context.Context) ([]dto.BuiltInWorkflowDTO, error) {
	var results []struct {
		AgentID     uuid.UUID
		AgentName   string
		WorkflowID  uuid.UUID
		WebAppID    uuid.UUID
		Description string
		AgentType   string
		Icon        *string
		IconType    *string
	}

	err := r.db.WithContext(ctx).
		Table("agents").
		Select(`
			agents.id as agent_id,
			agents.name as agent_name,
			agents.workflow_id as workflow_id,
			agents.web_app_id as web_app_id,
			agents.description as description,
			agents.agent_type as agent_type,
			agents.icon as icon,
			agents.icon_type as icon_type
			`).
		Joins("INNER JOIN workflows ON agents.workflow_id = workflows.id").
		Where("agents.tenant_id = ? AND agents.deleted_at IS NULL", builtInWorkflowTenantID).
		Where("workflows.tenant_id = ?", builtInWorkflowTenantID).
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query built-in workflows: %w", err)
	}

	dtos := make([]dto.BuiltInWorkflowDTO, 0, len(results))
	for _, result := range results {
		dtos = append(dtos, dto.BuiltInWorkflowDTO{
			Scenario:    r.deriveScenarioFromUUID(result.AgentID),
			AgentID:     result.AgentID,
			AgentName:   result.AgentName,
			WorkflowID:  result.WorkflowID,
			WebAppID:    result.WebAppID,
			Description: result.Description,
			AgentType:   result.AgentType,
			Icon:        result.Icon,
			IconType:    result.IconType,
		})
	}

	sortBuiltInWorkflowDTOs(dtos)
	return dtos, nil
}

func (r *builtInWorkflowRepository) getLegacyBuiltInWorkflowByID(ctx context.Context, id uuid.UUID) (*dto.BuiltInWorkflowDTO, error) {
	var result struct {
		AgentID     uuid.UUID
		AgentName   string
		WorkflowID  uuid.UUID
		WebAppID    uuid.UUID
		Description string
		AgentType   string
		Icon        *string
		IconType    *string
	}

	err := r.db.WithContext(ctx).
		Table("agents").
		Select(`
			agents.id as agent_id,
			agents.name as agent_name,
			agents.workflow_id as workflow_id,
			agents.web_app_id as web_app_id,
			agents.description as description,
			agents.agent_type as agent_type,
			agents.icon as icon,
			agents.icon_type as icon_type
			`).
		Joins("INNER JOIN workflows ON agents.workflow_id = workflows.id").
		Where("agents.id = ? AND agents.tenant_id = ? AND agents.deleted_at IS NULL", id, builtInWorkflowTenantID).
		Where("workflows.tenant_id = ?", builtInWorkflowTenantID).
		Limit(1).
		Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query built-in workflow by ID: %w", err)
	}
	if result.AgentID == uuid.Nil {
		return nil, fmt.Errorf("built-in workflow not found")
	}

	return &dto.BuiltInWorkflowDTO{
		Scenario:    r.deriveScenarioFromUUID(result.AgentID),
		AgentID:     result.AgentID,
		AgentName:   result.AgentName,
		WorkflowID:  result.WorkflowID,
		WebAppID:    result.WebAppID,
		Description: result.Description,
		AgentType:   result.AgentType,
		Icon:        result.Icon,
		IconType:    result.IconType,
	}, nil
}

func (r *builtInWorkflowRepository) getLegacyBuiltInWorkflowByScenario(ctx context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error) {
	expectedID := pkguuid.GenerateBuiltInWorkflowUUID(scenario)

	var result struct {
		AgentID     uuid.UUID
		AgentName   string
		WorkflowID  uuid.UUID
		WebAppID    uuid.UUID
		Description string
		AgentType   string
		Icon        *string
		IconType    *string
	}

	err := r.db.WithContext(ctx).
		Table("agents").
		Select(`
			agents.id as agent_id,
			agents.name as agent_name,
			agents.workflow_id as workflow_id,
			agents.web_app_id as web_app_id,
			agents.description as description,
			agents.agent_type as agent_type,
			agents.icon as icon,
			agents.icon_type as icon_type
			`).
		Joins("INNER JOIN workflows ON agents.workflow_id = workflows.id").
		Where("agents.id = ? AND agents.tenant_id = ? AND agents.deleted_at IS NULL", expectedID, builtInWorkflowTenantID).
		Where("workflows.tenant_id = ?", builtInWorkflowTenantID).
		Limit(1).
		Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query built-in workflow by scenario: %w", err)
	}
	if result.AgentID == uuid.Nil {
		return nil, fmt.Errorf("built-in workflow not found for scenario: %s", scenario)
	}

	return &dto.BuiltInWorkflowDTO{
		Scenario:    scenario,
		AgentID:     result.AgentID,
		AgentName:   result.AgentName,
		WorkflowID:  result.WorkflowID,
		WebAppID:    result.WebAppID,
		Description: result.Description,
		AgentType:   result.AgentType,
		Icon:        result.Icon,
		IconType:    result.IconType,
	}, nil
}

func (r *builtInWorkflowRepository) deriveScenarioFromUUID(agentID uuid.UUID) string {
	knownScenarios := []string{"global_chat", "bi_chat", "imagegen_chat"}
	for _, scenario := range knownScenarios {
		expectedID := pkguuid.GenerateBuiltInWorkflowUUID(scenario)
		if expectedID == agentID {
			return scenario
		}
	}
	return ""
}

func sortBuiltInWorkflowDTOs(dtos []dto.BuiltInWorkflowDTO) {
	scenarioOrder := map[string]int{
		"global_chat":   0,
		"bi_chat":       1,
		"imagegen_chat": 2,
	}
	sort.SliceStable(dtos, func(i, j int) bool {
		leftOrder, leftKnown := scenarioOrder[dtos[i].Scenario]
		rightOrder, rightKnown := scenarioOrder[dtos[j].Scenario]
		if leftKnown && rightKnown {
			return leftOrder < rightOrder
		}
		if leftKnown {
			return true
		}
		if rightKnown {
			return false
		}
		return dtos[i].Scenario < dtos[j].Scenario
	})
}
