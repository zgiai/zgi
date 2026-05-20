package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/dto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkflowRepository_GetLatestPublishedWorkflow_IgnoresContextTenantScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	createWorkflowRepositoryTestSchema(t, db)

	repo := NewWorkflowRepository(db)

	agentID := "7b59b3f3-e232-4c88-9b33-7d891241989c"
	workspaceID := "e932b153-a80b-48b9-aa18-eced2bfd2fcf"
	createdBy := "c6715e43-c82c-46af-9eba-acb3c7f2945f"

	oldPublished := &Workflow{
		ID:        "d05d5528-2d7b-48ab-a82b-886dde76b012",
		TenantID:  workspaceID,
		AppID:     agentID,
		AgentID:   agentID,
		Type:      dto.WorkflowType("chat"),
		Version:   "20260312152130",
		CreatedBy: createdBy,
		CreatedAt: time.Date(2026, 3, 12, 7, 21, 30, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 12, 7, 21, 30, 0, time.UTC),
	}
	newPublished := &Workflow{
		ID:        "a9f02ce5-e9d3-421f-9935-0e6483362057",
		TenantID:  workspaceID,
		AppID:     agentID,
		AgentID:   agentID,
		Type:      dto.WorkflowType("chat"),
		Version:   "20260315010337",
		CreatedBy: createdBy,
		CreatedAt: time.Date(2026, 3, 14, 17, 3, 37, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 14, 17, 3, 37, 0, time.UTC),
	}
	draft := &Workflow{
		ID:        "9058e421-9e15-4305-89c5-3dfe0da2f570",
		TenantID:  workspaceID,
		AppID:     agentID,
		AgentID:   agentID,
		Type:      dto.WorkflowType("chat"),
		Version:   "draft",
		CreatedBy: createdBy,
		CreatedAt: time.Date(2026, 3, 15, 1, 5, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 15, 1, 5, 0, 0, time.UTC),
	}

	require.NoError(t, db.Create(oldPublished).Error)
	require.NoError(t, db.Create(newPublished).Error)
	require.NoError(t, db.Create(draft).Error)

	workflow, err := repo.GetLatestPublishedWorkflow(context.Background(), agentID)
	require.NoError(t, err)
	require.NotNil(t, workflow)
	require.Equal(t, newPublished.ID, workflow.ID)
	require.Equal(t, newPublished.Version, workflow.Version)
}

func TestWorkflowRepository_GetByAgentID_MapsUnsetTenantToBuiltInTenant(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	createWorkflowRepositoryTestSchema(t, db)

	repo := NewWorkflowRepository(db)
	agentID := "77777777-7777-7777-7777-777777777777"
	expected := &Workflow{
		ID:        "99999999-9999-9999-9999-999999999999",
		TenantID:  builtInWorkflowTenantID,
		AppID:     agentID,
		AgentID:   agentID,
		Type:      dto.WorkflowType("chat"),
		Version:   "202603180001",
		Graph:     `{"nodes":[]}`,
		Features:  `{}`,
		CreatedBy: "account-1",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, db.Create(expected).Error)

	workflow, err := repo.GetByAgentID(context.Background(), "", agentID)
	require.NoError(t, err)
	require.NotNil(t, workflow)
	require.Equal(t, expected.ID, workflow.ID)
	require.Equal(t, builtInWorkflowTenantID, workflow.TenantID)
}

func createWorkflowRepositoryTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			app_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			type TEXT NOT NULL,
			version TEXT NOT NULL,
			version_uuid TEXT,
			graph TEXT,
			features TEXT,
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_by TEXT,
			updated_at DATETIME NOT NULL,
			environment_variables TEXT NOT NULL DEFAULT '[]',
			conversation_variables TEXT NOT NULL DEFAULT '[]',
			internal BOOLEAN NOT NULL DEFAULT 0
		)
	`).Error)
}
