package migrations

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRestoreKnowledgeGraphMigrationDefinesStableContract(t *testing.T) {
	sql := compactSQL(restoreKnowledgeGraphSQL)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS graph_revision bigint NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS graph_visibility_revision bigint NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS retrieval_enabled boolean NOT NULL DEFAULT true",
		"CREATE TABLE IF NOT EXISTS public.graphflow_runs",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_runs_dataset_idempotency",
		"lease_expires_at timestamptz",
		"CREATE TABLE IF NOT EXISTS public.graph_outbox_events",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_outbox_active_aggregate",
		"WHERE status IN ('pending', 'processing')",
		"ADD COLUMN IF NOT EXISTS run_id uuid",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_tasks_run_type",
		"ADD COLUMN IF NOT EXISTS evidence_fingerprint varchar(128) NOT NULL DEFAULT ''",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_mentions_document_evidence",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_triple_mentions_document_evidence",
		"ADD COLUMN IF NOT EXISTS relationship_id uuid",
		"SET document_id = segment.document_id, organization_id = segment.organization_id",
		"SET source_ref_id = ref.id",
		"SET relationship_id = relationship.id",
		"ADD COLUMN IF NOT EXISTS active_source_count integer NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS active_weight integer NOT NULL DEFAULT 0",
		"SET graph_status = CASE WHEN enable_graph_flow THEN 'waiting_content' ELSE 'disabled' END",
		"SET retrieval_enabled = (status <> 'disabled')",
		"SET active_source_count =",
		"SET active_weight =",
		"SET embedding_model_provider = dataset.embedding_model_provider",
		"WHERE enable_graph_flow = true",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("restore knowledge graph SQL missing %q: %s", want, sql)
		}
	}

	entityMentionsStart := strings.Index(sql, "ALTER TABLE public.kb_entity_mentions")
	tripleMentionsStart := strings.Index(sql, "ALTER TABLE public.kb_triple_mentions")
	tripleMentionsEnd := strings.Index(sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_triple_mentions_document_evidence")
	if entityMentionsStart < 0 || tripleMentionsStart < 0 || tripleMentionsEnd < 0 {
		t.Fatal("restore knowledge graph SQL is missing mention table migration sections")
	}
	if strings.Contains(sql[entityMentionsStart:tripleMentionsStart], "relationship_id") {
		t.Fatal("entity mentions must not define relationship_id")
	}
	if !strings.Contains(sql[tripleMentionsStart:tripleMentionsEnd], "ADD COLUMN IF NOT EXISTS relationship_id uuid") {
		t.Fatal("triple mentions must define relationship_id before backfill")
	}
}

func TestRestoreKnowledgeGraphModelsSupportSQLite(t *testing.T) {
	dsn := fmt.Sprintf("file:restore-knowledge-graph-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&graphmodel.GraphFlowRun{}, &graphmodel.GraphOutboxEvent{}); err != nil {
		t.Fatal(err)
	}

	datasetID := uuid.New()
	run := &graphmodel.GraphFlowRun{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		DatasetID:      datasetID,
		GraphRevision:  1,
		Trigger:        "test",
		Mode:           graphmodel.GraphFlowRunModeBuild,
		IdempotencyKey: "dataset:test:build",
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != graphmodel.GraphFlowRunStatusPending {
		t.Fatalf("run status = %q, want %q", run.Status, graphmodel.GraphFlowRunStatusPending)
	}

	event := &graphmodel.GraphOutboxEvent{
		ID:             uuid.New(),
		OrganizationID: run.OrganizationID,
		DatasetID:      datasetID,
		RunID:          &run.ID,
		EventType:      graphmodel.GraphOutboxEventRun,
		AggregateKey:   run.ID.String(),
		Payload:        map[string]any{"run_id": run.ID.String()},
		AvailableAt:    time.Now().UTC(),
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatal(err)
	}
	if event.Status != graphmodel.GraphOutboxStatusPending {
		t.Fatalf("event status = %q, want %q", event.Status, graphmodel.GraphOutboxStatusPending)
	}
}
