package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

func normalizeMode(agentType string) string {
	m := strings.TrimSpace(agentType)
	switch strings.ToUpper(m) {
	case "AGENT":
		return "AGENT"
	case "CONVERSATIONAL_AGENT", "CONVERSATIONAL-AGENT":
		return "CONVERSATIONAL_AGENT"
	case "WORKFLOW":
		return "WORKFLOW"
	case "CONVERSATIONAL_WORKFLOW":
		return "CONVERSATIONAL_WORKFLOW"
	default:
		return "AGENT"
	}
}

func (s *agentsService) hasPublishedVersion(ctx context.Context, ag *Agent) (bool, error) {
	if ag == nil {
		return false, fmt.Errorf("agent is required")
	}
	if ag.AgentsType == "AGENT" {
		return s.agentsRepo.HasPublishedAgentVersion(ctx, ag.ID.String())
	}
	return s.agentsRepo.HasPublishedWorkflow(ctx, ag.ID.String())
}

func changeWorkflowType(workflowType string) dto.WorkflowType {
	m := strings.TrimSpace(workflowType)
	switch strings.ToUpper(m) {
	case "WORKFLOW":
		return dto.WorkflowTypeWorkflow
	case "CONVERSATIONAL_WORKFLOW":
		return dto.WorkflowTypeChat
	default:
		return dto.WorkflowTypeWorkflow
	}
}

func parseUUID(id string) uuid.UUID {
	v, _ := uuid.Parse(id)
	return v
}

func uuidPtrToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	value := id.String()
	return &value
}
