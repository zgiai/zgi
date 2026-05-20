package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/pkg/logger"
)

// BuiltInWorkflowService defines the interface for built-in workflow business logic operations
// Requirements: 3.1, 3.2, 3.3
type BuiltInWorkflowService interface {
	// GetAllBuiltInWorkflows retrieves all built-in workflows
	// Requirement 3.1, 3.4: Query all internal workflows
	GetAllBuiltInWorkflows(ctx context.Context) ([]dto.BuiltInWorkflowDTO, error)

	// GetBuiltInWorkflowByScenario retrieves a built-in workflow by scenario name
	// Requirement 3.1, 3.2: Query by scenario with validation
	GetBuiltInWorkflowByScenario(ctx context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error)

	// GetBuiltInWorkflowByID retrieves a built-in workflow by agent ID
	// Requirement 3.1: Query by ID
	GetBuiltInWorkflowByID(ctx context.Context, id uuid.UUID) (*dto.BuiltInWorkflowDTO, error)
}

// builtInWorkflowService implements BuiltInWorkflowService interface
type builtInWorkflowService struct {
	repo BuiltInWorkflowRepository
}

// NewBuiltInWorkflowService creates a new BuiltInWorkflowService instance
func NewBuiltInWorkflowService(repo BuiltInWorkflowRepository) BuiltInWorkflowService {
	return &builtInWorkflowService{
		repo: repo,
	}
}

// GetAllBuiltInWorkflows retrieves all built-in workflows
// Requirement 3.1, 3.4: Query all internal workflows
func (s *builtInWorkflowService) GetAllBuiltInWorkflows(ctx context.Context) ([]dto.BuiltInWorkflowDTO, error) {
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

	logger.Info("Successfully retrieved built-in workflows", "count", len(workflows))
	return workflows, nil
}

// GetBuiltInWorkflowByScenario retrieves a built-in workflow by scenario name
// Requirement 3.1, 3.2, 3.3: Query by scenario with validation and error handling
func (s *builtInWorkflowService) GetBuiltInWorkflowByScenario(ctx context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error) {
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

	logger.Info("Successfully retrieved built-in workflow", "scenario", scenario, "agentID", workflow.AgentID)
	return workflow, nil
}

// GetBuiltInWorkflowByID retrieves a built-in workflow by agent ID
// Requirement 3.1: Query by ID
func (s *builtInWorkflowService) GetBuiltInWorkflowByID(ctx context.Context, id uuid.UUID) (*dto.BuiltInWorkflowDTO, error) {
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

	logger.Info("Successfully retrieved built-in workflow", "id", id, "scenario", workflow.Scenario)
	return workflow, nil
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
