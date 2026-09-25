package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

// The runtime projection schema is hand-written because the dataset model
// carries PostgreSQL-only defaults (uuid_generate_v4, CURRENT_TIMESTAMP(0)) that
// SQLite rejects during AutoMigrate.
const memoryProjectionDatasetsSchema = `
CREATE TABLE datasets (
	id text PRIMARY KEY,
	organization_id text NOT NULL,
	workspace_id text,
	name text NOT NULL,
	description text,
	provider text NOT NULL DEFAULT 'vendor',
	permission text NOT NULL DEFAULT 'all_team',
	enable_graph_flow numeric DEFAULT false,
	graph_status text NOT NULL DEFAULT 'disabled',
	graph_revision integer NOT NULL DEFAULT 0,
	graph_available_revision integer,
	graph_projected_revision integer NOT NULL DEFAULT 0,
	graph_visibility_revision integer NOT NULL DEFAULT 0,
	graph_projected_visibility_revision integer NOT NULL DEFAULT 0,
	graph_current_run_id text,
	graph_progress integer NOT NULL DEFAULT 0,
	graph_error_code text,
	graph_error_message text,
	graph_ready_at datetime,
	graph_updated_at datetime,
	created_by text NOT NULL,
	created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_by text,
	updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
	owner text,
	embedding_model text,
	embedding_model_provider text,
	entity_model text,
	entity_model_provider text,
	collection_binding_id text,
	retrieval_config text,
	icon_type text,
	icon text,
	icon_background text,
	process_rule text
)`

const memoryProjectionStatesSchema = `
CREATE TABLE digital_human_memory_states (
	id text PRIMARY KEY,
	organization_id text NOT NULL,
	workspace_id text NOT NULL,
	digital_human_id text NOT NULL,
	memory_epoch integer NOT NULL DEFAULT 0,
	config_revision text NOT NULL DEFAULT '',
	auto_extraction_enabled numeric NOT NULL DEFAULT 0,
	extraction_cutoff_at datetime,
	memory_dataset_id text,
	created_at datetime,
	updated_at datetime
)`

func newMemoryProjectionTestDB(t *testing.T) (*GormRepository, *Service) {
	t.Helper()
	service, repo := newTestService(t)
	if err := repo.db.Exec(memoryProjectionDatasetsSchema).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Exec(memoryProjectionStatesSchema).Error; err != nil {
		t.Fatal(err)
	}
	return repo, service
}

// The hidden projection is runtime infrastructure. It must never be offered as a
// bindable knowledge base, because the console persists whatever the candidate
// picker returns.
func TestKnowledgeCandidatesHideRuntimeManagedProjection(t *testing.T) {
	t.Parallel()
	repo, service := newMemoryProjectionTestDB(t)
	ctx := t.Context()
	scope := testScope()
	detail := createPublishableDigitalHuman(t, service, scope, "candidates")

	now := time.Now().UTC()
	for _, row := range []struct {
		id       string
		name     string
		provider string
	}{
		{id: uuid.NewString(), name: "Sales handbook", provider: "vendor"},
		{id: uuid.NewString(), name: "DigitalHuman memory projection", provider: datasetmodel.ProviderDigitalHumanMemory},
	} {
		if err := repo.db.Exec(
			"INSERT INTO datasets (id, organization_id, workspace_id, name, description, provider, permission, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, '', ?, 'all_team', ?, ?, ?)",
			row.id, scope.OrganizationID.String(), scope.WorkspaceID.String(), row.name, row.provider, scope.ActorID.String(), now, now,
		).Error; err != nil {
			t.Fatal(err)
		}
	}

	candidates, err := repo.ListResourceCandidates(ctx, scope, detail.Definition.ID, ResourceCandidateQuery{
		Type: resourcebindings.BindingTypeKnowledgeDataset, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].DisplayName != "Sales handbook" {
		t.Fatalf("knowledge candidates = %#v, want only the user knowledge base", candidates)
	}
}
