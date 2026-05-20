package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkguuid "github.com/zgiai/ginext/pkg/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuiltInWorkflowRepository_UsesLocalBuiltInWorkflowTables(t *testing.T) {
	db := setupBuiltInWorkflowRepositoryDB(t)
	seedLegacyBuiltInWorkflow(t, db, "imagegen_chat", "Legacy Image Generation")
	seedLegacyBuiltInWorkflow(t, db, "global_chat", "Legacy Global Chat")
	seedLegacyBuiltInWorkflow(t, db, "bi_chat", "Legacy BI Chat")

	repo := NewBuiltInWorkflowRepository(db)

	workflow, err := repo.GetBuiltInWorkflowByScenario(context.Background(), "global_chat")
	require.NoError(t, err)
	require.Equal(t, "Legacy Global Chat", workflow.AgentName)

	workflows, err := repo.GetAllBuiltInWorkflows(context.Background())
	require.NoError(t, err)
	require.Len(t, workflows, 3)
	require.Equal(t, []string{"global_chat", "bi_chat", "imagegen_chat"}, []string{
		workflows[0].Scenario,
		workflows[1].Scenario,
		workflows[2].Scenario,
	})
	require.Equal(t, "Legacy Global Chat", workflows[0].AgentName)
}

func setupBuiltInWorkflowRepositoryDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE agents (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			agent_type TEXT NOT NULL,
			icon_type TEXT NULL,
			icon TEXT NULL,
			workflow_id TEXT NULL,
			web_app_id TEXT NOT NULL UNIQUE,
			deleted_at DATETIME NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			app_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			type TEXT NOT NULL,
			version TEXT NOT NULL,
			graph TEXT,
			features TEXT,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			environment_variables TEXT NOT NULL DEFAULT '[]',
			conversation_variables TEXT NOT NULL DEFAULT '[]',
			internal BOOLEAN NOT NULL DEFAULT FALSE
		)
	`).Error)
	return db
}

func seedLegacyBuiltInWorkflow(t *testing.T, db *gorm.DB, scenario, name string) {
	t.Helper()

	agentID := pkguuid.GenerateBuiltInWorkflowUUID(scenario)
	workflowID := pkguuid.GenerateBuiltInWorkflowUUID(scenario + "_workflow")
	webAppID := pkguuid.GenerateBuiltInWorkflowUUID(scenario + "_webapp")
	now := time.Now().UTC()

	require.NoError(t, db.Exec(`
		INSERT INTO agents (
			id, tenant_id, name, description, agent_type, workflow_id, web_app_id
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, agentID.String(), builtInWorkflowTenantID, name, "legacy built-in workflow", "CONVERSATIONAL_WORKFLOW", workflowID.String(), webAppID.String()).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO workflows (
			id, tenant_id, app_id, agent_id, type, version, graph, features,
			created_by, created_at, updated_at, environment_variables, conversation_variables, internal
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, workflowID.String(), builtInWorkflowTenantID, agentID.String(), agentID.String(), "chat", "1", "{}", "{}", builtInWorkflowTenantID, now, now, "[]", "[]", true).Error)
}
