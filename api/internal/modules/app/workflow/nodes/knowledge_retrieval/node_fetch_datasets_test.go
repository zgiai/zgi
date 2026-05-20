package knowledgeretrieval

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	datasetmodel "github.com/zgiai/ginext/internal/modules/dataset/model"
)

func TestFetchAvailableDatasetsUsesWorkspaceScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE datasets (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			workspace_id TEXT,
			name TEXT NOT NULL,
			description TEXT,
			provider TEXT NOT NULL,
			enable_graph_flow BOOLEAN NOT NULL DEFAULT FALSE,
			created_by TEXT NOT NULL,
			created_at DATETIME,
			updated_by TEXT,
			updated_at DATETIME,
			owner TEXT,
			embedding_model TEXT,
			embedding_model_provider TEXT,
			entity_model TEXT,
			entity_model_provider TEXT,
			collection_binding_id TEXT,
			retrieval_config TEXT,
			icon_type TEXT,
			icon TEXT,
			icon_background TEXT,
			process_rule TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create datasets table: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE documents (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			dataset_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			data_source_type TEXT NOT NULL,
			batch TEXT NOT NULL,
			name TEXT NOT NULL,
			created_from TEXT NOT NULL,
			created_by TEXT NOT NULL,
			indexing_status TEXT NOT NULL,
			enabled BOOLEAN NOT NULL,
			archived BOOLEAN NOT NULL,
			doc_form TEXT NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create documents table: %v", err)
	}

	datasetID := "dataset-1"
	if err := db.Exec(`
		INSERT INTO datasets (id, organization_id, workspace_id, name, provider, enable_graph_flow, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, datasetID, "org-1", "ws-1", "Knowledge Base", "vendor", false, "account-1").Error; err != nil {
		t.Fatalf("insert dataset: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO documents (id, organization_id, dataset_id, position, data_source_type, batch, name, created_from, created_by, indexing_status, enabled, archived, doc_form)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "doc-1", "org-1", datasetID, 1, "upload_file", "batch-1", "Doc 1", "api", "account-1", datasetmodel.DocumentStatusCompleted, true, false, "text_model").Error; err != nil {
		t.Fatalf("insert document: %v", err)
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			TenantID: "ws-1",
		},
		NodeData: NodeData{
			DatasetIds: []string{datasetID},
		},
		db: db,
	}

	datasets, err := node.fetchAvailableDatasets()
	if err != nil {
		t.Fatalf("fetch available datasets: %v", err)
	}

	if len(datasets) != 1 {
		t.Fatalf("expected 1 dataset when tenant id represents workspace_id, got %d", len(datasets))
	}
}
