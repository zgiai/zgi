package workflow

import (
	"context"
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
	if err := validateWorkflowConnectorNodesConfigured(draftWorkflow.GetGraphDict()); err != nil {
		return nil, fmt.Errorf("workflow is not publishable: %w", err)
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

	connectorBindings, err := s.workflowConnectorBindingRows(ctx, newWorkflow, newWorkflow.GetGraphDict(), accountID)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow connector dependencies: %w", err)
	}

	// 5. Save the immutable graph version and its exact connector dependency
	// index, revoke the superseded canonical snapshot, and advance the runtime
	// head atomically. Historical graph rows remain addressable for inspection,
	// but are not authorized Connector runtime heads.
	if err := s.persistPublishedWorkflowWithBindings(ctx, newWorkflow, draftWorkflow, connectorBindings); err != nil {
		logger.Error("Failed to create workflow version", err)
		return nil, fmt.Errorf("failed to create workflow version: %w", err)
	}

	logger.Info("Workflow published successfully", "version", versionTimestamp)

	return &PublishWorkflowResponse{
		WorkflowID:  newWorkflow.ID,
		VersionUUID: versionUUID.String(),
		Version:     versionTimestamp,
		CreatedAt:   newWorkflow.CreatedAt.Format(time.RFC3339),
	}, nil
}

// GetWorkflowByVersionUUID retrieves a workflow by its version UUID
func (s *WorkflowService) GetWorkflowByVersionUUID(ctx context.Context, versionUUID string) (interface{}, error) {
	return s.repo.GetByVersionUUID(ctx, versionUUID)
}

// GetPublishedVersions retrieves all published versions for an agent
func (s *WorkflowService) GetPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*Workflow, int64, error) {
	return s.repo.GetPublishedVersions(ctx, agentID, limit, offset)
}
