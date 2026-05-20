package seeders

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
	"gopkg.in/yaml.v3"
)

const (
	// BuiltInTenantID is the special tenant ID for built-in workflows
	BuiltInTenantID = "00000000-0000-0000-0000-000000000000"
)

var builtInWorkflowSeedScenarios = []string{"bi_chat", "global_chat", "imagegen_chat"}

// BuiltInWorkflowSeedScenarios returns the scenarios managed by the local seed set.
func BuiltInWorkflowSeedScenarios() []string {
	return append([]string(nil), builtInWorkflowSeedScenarios...)
}

// BuiltInWorkflowSeedAgentIDs returns deterministic agent IDs for local built-in workflows.
func BuiltInWorkflowSeedAgentIDs() []string {
	ids := make([]string, 0, len(builtInWorkflowSeedScenarios))
	for _, scenario := range builtInWorkflowSeedScenarios {
		ids = append(ids, pkguuid.GenerateBuiltInWorkflowUUID(scenario).String())
	}
	return ids
}

// WorkflowDefinition represents a single workflow definition from YAML
type WorkflowDefinition struct {
	Scenario    string `yaml:"scenario"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	AgentType   string `yaml:"agent_type"`
	IconType    string `yaml:"icon_type"`
	Icon        string `yaml:"icon"`
	EnableAPI   bool   `yaml:"enable_api"`
	IsPublic    bool   `yaml:"is_public"`
	IsUniversal bool   `yaml:"is_universal"`
	Internal    bool   `yaml:"internal"`
	Version     string `yaml:"version"` // Workflow version (default: "1.0.0")
}

// WorkflowDefinitions represents the root structure of the definitions YAML file
type WorkflowDefinitions struct {
	Workflows []WorkflowDefinition `yaml:"workflows"`
}

// SeedBuiltInWorkflows seeds built-in workflows from YAML and JSON files
// This function reads workflow definitions from definitions.yaml and loads
// corresponding graph and features JSON files for each workflow
func SeedBuiltInWorkflows(ctx context.Context, db *sql.DB) error {
	logger.Info("Seeding built-in workflows...")

	baseDir := "seeds/00_base/workflows"

	// Read definitions YAML file
	defPath := path.Join(baseDir, "definitions.yaml")
	defData, err := seedFS.ReadFile(defPath)
	if err != nil {
		return fmt.Errorf("failed to read definitions: %w", err)
	}

	// Parse YAML
	var defs WorkflowDefinitions
	if err := yaml.Unmarshal(defData, &defs); err != nil {
		return fmt.Errorf("failed to parse definitions: %w", err)
	}

	// Process each workflow
	successCount := 0
	failCount := 0
	for _, def := range defs.Workflows {
		if err := seedWorkflow(ctx, db, baseDir, def); err != nil {
			logger.Error(fmt.Sprintf("Failed to seed workflow %s", def.Scenario), err)
			failCount++
			// Continue with other workflows
			continue
		}
		logger.Info("✓ Seeded workflow", "scenario", def.Scenario)
		successCount++
	}

	logger.Info("Built-in workflows seeding completed",
		"success", successCount,
		"failed", failCount,
		"total", len(defs.Workflows))

	return nil
}

// seedWorkflow seeds a single workflow into the database
// It generates deterministic UUIDs, reads JSON configuration files,
// and inserts/updates the agent and workflow records
func seedWorkflow(ctx context.Context, db *sql.DB, baseDir string, def WorkflowDefinition) error {
	// Generate deterministic UUIDs using the UUID utility package
	agentID := pkguuid.GenerateBuiltInWorkflowUUID(def.Scenario)
	workflowID := pkguuid.GenerateBuiltInWorkflowUUID(def.Scenario + "_workflow")
	webAppID := pkguuid.GenerateBuiltInWorkflowUUID(def.Scenario + "_webapp")

	// Read graph JSON file
	graphPath := path.Join(baseDir, def.Scenario+"_graph.json")
	graphData, err := seedFS.ReadFile(graphPath)
	if err != nil {
		return fmt.Errorf("failed to read graph file %s: %w", graphPath, err)
	}

	// Validate graph JSON
	var graphObj interface{}
	if err := json.Unmarshal(graphData, &graphObj); err != nil {
		return fmt.Errorf("invalid graph JSON in %s: %w", graphPath, err)
	}

	// Read features JSON file
	featuresPath := path.Join(baseDir, def.Scenario+"_features.json")
	featuresData, err := seedFS.ReadFile(featuresPath)
	if err != nil {
		return fmt.Errorf("failed to read features file %s: %w", featuresPath, err)
	}

	// Validate features JSON
	var featuresObj interface{}
	if err := json.Unmarshal(featuresData, &featuresObj); err != nil {
		return fmt.Errorf("invalid features JSON in %s: %w", featuresPath, err)
	}

	// Begin transaction for atomicity
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert or update agent record
	agentSQL := `
		INSERT INTO agents (
			id, tenant_id, name, description, agent_type, icon_type, icon,
			agents_model_config_id, workflow_id, enable_api, is_public, is_universal,
			internal, web_app_id, created_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NULL, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			agent_type = EXCLUDED.agent_type,
			icon_type = EXCLUDED.icon_type,
			icon = EXCLUDED.icon,
			workflow_id = EXCLUDED.workflow_id,
			enable_api = EXCLUDED.enable_api,
			is_public = EXCLUDED.is_public,
			is_universal = EXCLUDED.is_universal,
			updated_at = NOW()
	`

	_, err = tx.ExecContext(ctx, agentSQL,
		agentID, BuiltInTenantID, def.Name, def.Description, def.AgentType,
		def.IconType, def.Icon, workflowID, def.EnableAPI, def.IsPublic,
		def.IsUniversal, def.Internal, webAppID, BuiltInTenantID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert agent: %w", err)
	}

	// Generate timestamp version for published workflow
	timestampVersion := fmt.Sprintf("%d", time.Now().Unix())

	// Insert or update workflow SQL template
	workflowSQL := `
		INSERT INTO workflows (
			id, tenant_id, app_id, type, version, graph, features,
			created_by, created_at, updated_at, environment_variables,
			conversation_variables, internal, agent_id
		) VALUES (
			$1, $2, $3, 'chat', $4, $5, $6, $7, NOW(), NOW(), '[]', '[]', true, $8
		)
		ON CONFLICT (id) DO UPDATE SET
			graph = EXCLUDED.graph,
			features = EXCLUDED.features,
			version = EXCLUDED.version,
			updated_at = NOW()
	`

	// 1. Insert draft version workflow
	draftWorkflowID := pkguuid.GenerateBuiltInWorkflowUUID(def.Scenario + "_workflow_draft")
	_, err = tx.ExecContext(ctx, workflowSQL,
		draftWorkflowID, BuiltInTenantID, agentID, "draft", string(graphData), string(featuresData),
		BuiltInTenantID, agentID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert draft workflow: %w", err)
	}

	// 2. Insert published version workflow (with timestamp)
	_, err = tx.ExecContext(ctx, workflowSQL,
		workflowID, BuiltInTenantID, agentID, timestampVersion, string(graphData), string(featuresData),
		BuiltInTenantID, agentID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert workflow: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
