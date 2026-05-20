package workflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkflowExportData represents the top-level YAML structure for workflow export
type WorkflowExportData struct {
	Kind         string             `yaml:"kind"`
	Version      string             `yaml:"version"`
	ExportedFrom string             `yaml:"exported_from"`
	ExportedAt   string             `yaml:"exported_at"`
	App          ExportAppInfo      `yaml:"app"`
	Workflow     ExportWorkflowData `yaml:"workflow"`
}

// ExportAppInfo contains application metadata for export
type ExportAppInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
	IconType    string `yaml:"icon_type"`
	Mode        string `yaml:"mode"`
}

// ExportWorkflowData contains the complete workflow definition for export
type ExportWorkflowData struct {
	ConversationVariables []interface{}          `yaml:"conversation_variables"`
	EnvironmentVariables  []interface{}          `yaml:"environment_variables"`
	Features              map[string]interface{} `yaml:"features"`
	Graph                 map[string]interface{} `yaml:"graph"`
}

const exportFormatVersion = "1.0.0"

// ExportWorkflow exports a workflow as YAML bytes with sanitized sensitive data.
// version can be "draft" or "published".
func (s *WorkflowService) ExportWorkflow(
	ctx context.Context,
	agentID string,
	version string,
) ([]byte, string, error) {
	agent, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, "", fmt.Errorf("agent not found: %w", err)
	}

	var workflow *Workflow
	if version == "published" {
		workflow, err = s.repo.GetLatestPublishedWorkflow(ctx, agentID)
	} else {
		workflow, err = s.repo.GetDraftWorkflow(ctx, agentID)
	}
	if err != nil {
		return nil, "", fmt.Errorf("workflow not found: %w", err)
	}

	graphDict := workflow.GetGraphDict()
	featuresDict := workflow.GetFeaturesDict()

	SanitizeGraph(graphDict)

	icon := ""
	if agent.Icon != nil {
		icon = *agent.Icon
	}
	iconType := ""
	if agent.IconType != nil {
		iconType = *agent.IconType
	}

	exportData := WorkflowExportData{
		Kind:         "app",
		Version:      exportFormatVersion,
		ExportedFrom: "zgi",
		ExportedAt:   time.Now().UTC().Format(time.RFC3339),
		App: ExportAppInfo{
			Name:        agent.Name,
			Description: agent.Description,
			Icon:        icon,
			IconType:    iconType,
			Mode:        MapAgentTypeToMode(agent.AgentsType),
		},
		Workflow: ExportWorkflowData{
			ConversationVariables: workflow.GetConversationVariablesDict(),
			EnvironmentVariables:  workflow.GetEnvironmentVariablesDict(),
			Features:              featuresDict,
			Graph:                 graphDict,
		},
	}

	data, err := yaml.Marshal(exportData)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal yaml: %w", err)
	}

	// Use workflow name from export data (app.name in YAML) for filename
	workflowName := strings.TrimSpace(exportData.App.Name)
	if workflowName == "" {
		workflowName = "workflow"
	}
	filename := fmt.Sprintf("%s.yml", SanitizeFilename(workflowName))
	return data, filename, nil
}

// SanitizeGraph removes sensitive information from graph nodes before export
func SanitizeGraph(graph map[string]interface{}) {
	nodes, ok := graph["nodes"].([]interface{})
	if !ok {
		return
	}
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		data, ok := nodeMap["data"].(map[string]interface{})
		if !ok {
			continue
		}

		delete(data, "api_key")
		delete(data, "credential_id")
		delete(data, "selected")
		delete(nodeMap, "selected")

		nodeType, _ := data["type"].(string)

		if nodeType == "http-request" {
			if auth, ok := data["authorization"].(map[string]interface{}); ok {
				delete(auth, "api_key")
				delete(auth, "header_value")
			}
		}

		if nodeType == "call-database" {
			if ds, ok := data["data_source"].(map[string]interface{}); ok {
				delete(ds, "password")
				delete(ds, "connection_string")
			}
		}

		if nodeType == "llm" {
			if model, ok := data["model"].(map[string]interface{}); ok {
				delete(model, "credential_id")
			}
		}
	}
}

// MapAgentTypeToMode converts internal agent type to export mode string
func MapAgentTypeToMode(agentType string) string {
	switch agentType {
	case "advanced-chat":
		return "advanced-chat"
	case "workflow":
		return "workflow"
	default:
		return agentType
	}
}

var filenameUnsafeChars = regexp.MustCompile(`[^\p{L}\p{N}\-_. ]`)
var filenameHasVisibleName = regexp.MustCompile(`[\p{L}\p{N}]`)

// SanitizeFilename removes characters that are unsafe for use in filenames
func SanitizeFilename(name string) string {
	safe := filenameUnsafeChars.ReplaceAllString(name, "_")
	safe = strings.TrimSpace(safe)
	if safe == "" || !filenameHasVisibleName.MatchString(safe) {
		safe = "workflow"
	}
	return safe
}
