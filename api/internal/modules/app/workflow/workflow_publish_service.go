package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// PublishWorkflowRequest represents the request to publish a workflow
type PublishWorkflowRequest struct {
	Description string `json:"description"` // Version description
}

// PublishWorkflowResponse represents the response after publishing
type PublishWorkflowResponse struct {
	WorkflowID  string `json:"workflow_id"`
	VersionUUID string `json:"version_uuid"`
	Version     string `json:"version"`
	CreatedAt   string `json:"created_at"`
}

// PublishWorkflowWithDescription publishes the current draft workflow as a new version
func (s *WorkflowService) PublishWorkflowWithDescription(ctx context.Context, tenantID, agentID, accountID, description string) (*PublishWorkflowResponse, error) {
	logger.Info("Publishing workflow", "agentID", agentID, "accountID", accountID)

	draftWorkflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		logger.Error("Failed to get draft workflow", err)
		return nil, fmt.Errorf("failed to get draft workflow: %w", err)
	}

	now := time.Now()
	versionTimestamp := now.Format("20060102150405")
	versionUUID := uuid.New()

	// Create new workflow version record
	newWorkflow := &Workflow{
		ID:                    uuid.New().String(),
		TenantID:              draftWorkflow.TenantID,
		AppID:                 draftWorkflow.AppID,
		AgentID:               draftWorkflow.AgentID,
		Type:                  draftWorkflow.Type,
		Version:               versionTimestamp,
		VersionUUID:           uuid.NullUUID{UUID: versionUUID, Valid: true},
		Graph:                 draftWorkflow.Graph,
		Features:              draftWorkflow.Features,
		EnvironmentVariables:  draftWorkflow.EnvironmentVariables,
		ConversationVariables: draftWorkflow.ConversationVariables,
		CreatedBy:             accountID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	// 5. Save the new version to database
	if err := s.repo.CreateWorkflow(ctx, newWorkflow); err != nil {
		logger.Error("Failed to create workflow version", err)
		return nil, fmt.Errorf("failed to create workflow version: %w", err)
	}

	// 6. Update agent's workflow_id to point to the new version
	if err := s.agentsRepo.UpdateWorkflowID(ctx, agentID, newWorkflow.ID); err != nil {
		logger.Error("Failed to update agent workflow_id", err)
		// Don't fail the whole operation, just log the error
	}

	// 7. Update agent's workflow_config.workflow_id
	if err := s.updateAgentWorkflowConfig(ctx, agentID, newWorkflow.ID); err != nil {
		logger.Error("Failed to update agent workflow_config", err)
		// Don't fail the whole operation, just log the error
	}

	logger.Info("Workflow published successfully", "version", versionTimestamp)

	return &PublishWorkflowResponse{
		WorkflowID:  newWorkflow.ID,
		VersionUUID: versionUUID.String(),
		Version:     versionTimestamp,
		CreatedAt:   newWorkflow.CreatedAt.Format(time.RFC3339),
	}, nil
}

// updateAgentWorkflowConfig updates the workflow_id in agent's workflow_config
func (s *WorkflowService) updateAgentWorkflowConfig(ctx context.Context, agentID, workflowID string) error {
	// Get current agent
	agent, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}

	// Parse existing workflow_config or create new one
	var workflowConfig map[string]interface{}
	if agent.WorkflowConfig != nil && *agent.WorkflowConfig != "" {
		if err := json.Unmarshal([]byte(*agent.WorkflowConfig), &workflowConfig); err != nil {
			logger.Warn("Failed to parse existing workflow_config, creating new one", "error", err)
			workflowConfig = make(map[string]interface{})
		}
	} else {
		workflowConfig = make(map[string]interface{})
	}

	// Update workflow_id in config
	workflowConfig["workflow_id"] = workflowID

	// Ensure other required fields exist with defaults
	if _, exists := workflowConfig["variable_config"]; !exists {
		workflowConfig["variable_config"] = make(map[string]interface{})
	}
	if _, exists := workflowConfig["history_window_size"]; !exists {
		workflowConfig["history_window_size"] = 0
	}
	if _, exists := workflowConfig["conversation_id"]; !exists {
		workflowConfig["conversation_id"] = ""
	}

	// Marshal back to JSON
	configJSON, err := json.Marshal(workflowConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow_config: %w", err)
	}

	// Update agent's workflow_config
	if err := s.agentsRepo.UpdateWorkflowConfig(ctx, agentID, string(configJSON)); err != nil {
		return fmt.Errorf("failed to update agent workflow_config: %w", err)
	}

	logger.Info("Updated agent workflow_config", "agentID", agentID, "workflowID", workflowID)
	return nil
}

// GetWorkflowByVersionUUID retrieves a workflow by its version UUID
func (s *WorkflowService) GetWorkflowByVersionUUID(ctx context.Context, versionUUID string) (interface{}, error) {
	return s.repo.GetByVersionUUID(ctx, versionUUID)
}

// GetPublishedVersions retrieves all published versions for an agent
func (s *WorkflowService) GetPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*Workflow, int64, error) {
	return s.repo.GetPublishedVersions(ctx, agentID, limit, offset)
}
