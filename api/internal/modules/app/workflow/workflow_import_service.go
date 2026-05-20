package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graphconfig"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ImportResult contains the result of a workflow import operation
type ImportResult struct {
	Success    bool            `json:"success"`
	AgentID    string          `json:"agent_id"`
	WorkflowID string          `json:"workflow_id"`
	Stats      ImportStats     `json:"stats"`
	Warnings   []ImportWarning `json:"warnings"`
}

// ImportStats contains statistics about the imported workflow
type ImportStats struct {
	NodeCount     int      `json:"node_count"`
	EdgeCount     int      `json:"edge_count"`
	NodeTypes     []string `json:"node_types"`
	VariableCount int      `json:"variable_count"`
}

// ImportWarning represents a non-fatal issue detected during import
type ImportWarning struct {
	Type    string `json:"type"`
	NodeID  string `json:"node_id,omitempty"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

// supportedExportVersions lists export format versions this system can import
var supportedExportVersions = map[string]bool{
	"1.0.0": true,
}

// ImportWorkflow parses a YAML file and creates a new agent with the workflow. Import only supports creating new agents.
func (s *WorkflowService) ImportWorkflow(
	ctx context.Context,
	tenantID string,
	accountID string,
	fileData []byte,
) (*ImportResult, error) {
	var exportData WorkflowExportData
	if err := yaml.Unmarshal(fileData, &exportData); err != nil {
		return nil, fmt.Errorf("invalid YAML file: %w", err)
	}

	if exportData.Kind != "app" {
		return nil, fmt.Errorf("invalid file: kind must be 'app', got '%s'", exportData.Kind)
	}
	if exportData.ExportedFrom != "zgi" {
		return nil, fmt.Errorf("unsupported export source: '%s'", exportData.ExportedFrom)
	}
	if exportData.Workflow.Graph == nil {
		return nil, fmt.Errorf("invalid file: workflow graph is missing")
	}

	if !supportedExportVersions[exportData.Version] {
		return nil, fmt.Errorf("unsupported export format version '%s', supported: 1.0.0", exportData.Version)
	}

	graph := exportData.Workflow.Graph
	var warnings []ImportWarning

	nodeWarnings := CheckNodeCompatibility(graph)
	if len(nodeWarnings) > 0 {
		return nil, fmt.Errorf("unsupported node type '%s' in import graph", nodeWarnings[0].Name)
	}
	warnings = append(warnings, nodeWarnings...)

	depWarnings := CheckDependencies(graph)
	warnings = append(warnings, depWarnings...)

	stats := CollectImportStats(graph)

	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize graph: %w", err)
	}
	featuresJSON, err := json.Marshal(exportData.Workflow.Features)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize features: %w", err)
	}
	convVarsJSON, err := json.Marshal(exportData.Workflow.ConversationVariables)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize conversation variables: %w", err)
	}
	envVarsJSON, err := json.Marshal(exportData.Workflow.EnvironmentVariables)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize environment variables: %w", err)
	}

	// Create a new agent from the import
	agentType := strings.ToUpper(exportData.App.Mode)
	if agentType == "" {
		agentType = "CHAT_AGENT" // Default fallback
	}

	newAgent := &agents.Agent{
		TenantID:    parseUUID(tenantID),
		Name:        exportData.App.Name,
		Description: exportData.App.Description,
		AgentsType:  normalizeMode(exportData.App.Mode),
		EnableAPI:   true,
		WebAppID:    uuid.New(),
	}
	if exportData.App.Icon != "" {
		newAgent.Icon = &exportData.App.Icon
	}
	if exportData.App.IconType != "" {
		iconType := exportData.App.IconType
		newAgent.IconType = &iconType
	} else if exportData.App.Icon != "" {
		// Backward compatibility: when icon exists but icon_type is missing, default to emoji
		iconType := "text"
		newAgent.IconType = &iconType
	}
	if uid, err := uuid.Parse(accountID); err == nil {
		newAgent.CreatedBy = &uid
	}

	if err := s.agentsRepo.Create(ctx, newAgent); err != nil {
		return nil, fmt.Errorf("failed to create agent during import: %w", err)
	}
	agentID := newAgent.ID.String()

	// Create a default draft workflow for this new agent so we can overwrite it
	syncReq := &dto.SyncDraftWorkflowRequest{
		Graph:    nil,
		Features: nil,
		Type:     changeWorkflowType(newAgent.AgentsType),
	}
	_, err = s.SyncDraftWorkflow(ctx, tenantID, agentID, syncReq, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize workflow for new agent: %w", err)
	}

	var workflow *Workflow
	workflow, err = s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft workflow for new agent: %w", err)
	}

	// Update agent's workflow_id column to point to the draft (matching CreateAgent behavior)
	if workflowUUID, err := uuid.Parse(workflow.ID); err == nil {
		if err := s.agentsRepo.UpdateWorkflowID(ctx, agentID, workflowUUID.String()); err != nil {
			// Log error but don't fail, as the workflow record itself is more important
			logger.WarnContext(ctx, "failed to update agent workflow id during import",
				err,
				zap.String("agent_id", agentID),
				zap.String("workflow_id", workflow.ID),
				zap.String("tenant_id", tenantID),
				zap.String("account_id", accountID),
			)
		}
	}

	workflow.Graph = string(graphJSON)
	workflow.Features = string(featuresJSON)
	workflow.ConversationVariables = string(convVarsJSON)
	workflow.EnvironmentVariables = string(envVarsJSON)

	if err := s.repo.Update(ctx, workflow); err != nil {
		return nil, fmt.Errorf("failed to save imported workflow: %w", err)
	}

	return &ImportResult{
		Success:    true,
		AgentID:    agentID,
		WorkflowID: workflow.ID,
		Stats:      stats,
		Warnings:   warnings,
	}, nil
}

// CheckNodeCompatibility verifies all node types in the graph are supported
func CheckNodeCompatibility(graph map[string]interface{}) []ImportWarning {
	var warnings []ImportWarning

	nodes, _ := graph["nodes"].([]interface{})
	for _, node := range nodes {
		nodeMap, _ := node.(map[string]interface{})
		nodeID, _ := nodeMap["id"].(string)
		data, _ := nodeMap["data"].(map[string]interface{})
		nodeType, _ := data["type"].(string)

		if nodeType == "" {
			continue
		}
		if nodeType == "note" {
			continue
		}
		if _, err := graphconfig.ParseNodeType(nodeType); err != nil {
			warnings = append(warnings, ImportWarning{
				Type:    "unsupported_node",
				NodeID:  nodeID,
				Name:    nodeType,
				Message: fmt.Sprintf("Node type '%s' is not supported and will be skipped during execution", nodeType),
			})
		}
	}

	return warnings
}

// CheckDependencies checks if imported nodes reference external resources that need reconfiguration
func CheckDependencies(graph map[string]interface{}) []ImportWarning {
	var warnings []ImportWarning
	datasources := map[string]bool{}
	knowledgeBases := map[string]bool{}

	nodes, _ := graph["nodes"].([]interface{})
	for _, node := range nodes {
		nodeMap, _ := node.(map[string]interface{})
		nodeID, _ := nodeMap["id"].(string)
		nodeType := workflowNodeType(nodeMap)
		data := workflowNodeData(nodeMap)

		switch nodeType {
		case "call-database":
			ds := workflowNestedMap(data, "data_source")
			if len(ds) == 0 {
				ds = workflowNestedMap(workflowNestedMap(data, "data"), "data_source")
			}
			dsName, _ := ds["name"].(string)
			if dsName == "" {
				dsName, _ = ds["id"].(string)
			}
			if dsName == "" {
				dsName = "database"
			}
			if !datasources[dsName] {
				datasources[dsName] = true
				warnings = append(warnings, ImportWarning{
					Type:    "datasource_requires_config",
					NodeID:  nodeID,
					Name:    dsName,
					Message: fmt.Sprintf("Data source '%s' needs to be configured after import", dsName),
				})
			}
		case "sql-generator":
			ds := workflowNestedMap(workflowNestedMap(data, "data_source"), "source")
			if len(ds) == 0 {
				ds = workflowNestedMap(workflowNestedMap(workflowNestedMap(data, "data"), "data_source"), "source")
			}
			dsName, _ := ds["name"].(string)
			if dsName == "" {
				dsName, _ = ds["id"].(string)
			}
			if dsName != "" && !datasources[dsName] {
				datasources[dsName] = true
				warnings = append(warnings, ImportWarning{
					Type:    "datasource_requires_config",
					NodeID:  nodeID,
					Name:    dsName,
					Message: fmt.Sprintf("Data source '%s' needs to be configured after import", dsName),
				})
			}

		case "knowledge-retrieval":
			if dataset, ok := data["dataset_ids"].([]interface{}); ok {
				for _, id := range dataset {
					if dsID, ok := id.(string); ok && !knowledgeBases[dsID] {
						knowledgeBases[dsID] = true
						warnings = append(warnings, ImportWarning{
							Type:    "knowledge_base_requires_config",
							NodeID:  nodeID,
							Name:    dsID,
							Message: "Knowledge base reference needs to be re-configured after import",
						})
					}
				}
			}
		}
	}

	return warnings
}

// CollectImportStats gathers statistics about the imported workflow graph
func CollectImportStats(graph map[string]interface{}) ImportStats {
	stats := ImportStats{}
	typeSet := map[string]bool{}

	nodes, _ := graph["nodes"].([]interface{})
	stats.NodeCount = len(nodes)
	for _, node := range nodes {
		nodeMap, _ := node.(map[string]interface{})
		data, _ := nodeMap["data"].(map[string]interface{})
		nodeType := workflowNodeType(nodeMap)
		if nodeType != "" {
			typeSet[nodeType] = true
		}

		// Count start node variables
		if nodeType == "start" {
			if vars, ok := data["variables"].([]interface{}); ok {
				stats.VariableCount += len(vars)
			}
		}
	}

	edges, _ := graph["edges"].([]interface{})
	stats.EdgeCount = len(edges)

	for t := range typeSet {
		stats.NodeTypes = append(stats.NodeTypes, t)
	}
	sort.Strings(stats.NodeTypes)

	return stats
}

func normalizeMode(agentType string) string {
	m := strings.TrimSpace(agentType)
	switch strings.ToUpper(m) {
	case "AGENT":
		return "AGENT"
	case "CONVERSATIONAL_AGENT", "CONVERSATIONAL-AGENT", "CHAT":
		return "CONVERSATIONAL_AGENT"
	case "WORKFLOW":
		return "WORKFLOW"
	case "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL-WORKFLOW", "ADVANCED-CHAT":
		return "CONVERSATIONAL_WORKFLOW"
	default:
		return "AGENT"
	}
}

func changeWorkflowType(workflowType string) dto.WorkflowType {
	m := strings.TrimSpace(workflowType)
	switch strings.ToUpper(m) {
	case "WORKFLOW":
		return dto.WorkflowTypeWorkflow
	case "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL-WORKFLOW", "ADVANCED-CHAT":
		return dto.WorkflowTypeChat
	default:
		return dto.WorkflowTypeWorkflow
	}
}

func parseUUID(id string) uuid.UUID {
	v, _ := uuid.Parse(id)
	return v
}

func workflowNodeType(node map[string]any) string {
	if node == nil {
		return ""
	}

	if data, ok := node["data"].(map[string]any); ok {
		if nodeType, ok := data["type"].(string); ok {
			nodeType = strings.TrimSpace(nodeType)
			if nodeType != "" {
				return nodeType
			}
		}
	}

	if nodeType, ok := node["type"].(string); ok {
		return strings.TrimSpace(nodeType)
	}

	return ""
}

func workflowNodeData(node map[string]any) map[string]any {
	if node == nil {
		return map[string]any{}
	}
	if data, ok := node["data"].(map[string]any); ok {
		return data
	}
	return map[string]any{}
}

func workflowNestedMap(source map[string]any, key string) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	if nested, ok := source[key].(map[string]any); ok {
		return nested
	}
	return map[string]any{}
}
