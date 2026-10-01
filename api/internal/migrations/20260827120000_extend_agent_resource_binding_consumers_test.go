package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestExtendAgentResourceBindingConsumersMigrationExecutes(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.agent_resource_bindings.*workflow_connector_binding_backfills.*idx_agent_resource_bindings_workflow_integration_unique").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upExtendAgentResourceBindingConsumers(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestExtendAgentResourceBindingConsumersMigrationContract(t *testing.T) {
	sql := compactSQL(extendAgentResourceBindingConsumersSQL)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS consumer_type varchar(16) NOT NULL DEFAULT 'agent'",
		"ADD COLUMN IF NOT EXISTS consumer_id uuid",
		"ADD COLUMN IF NOT EXISTS binding_instance_id varchar(255) NOT NULL DEFAULT ''",
		"SET consumer_type = 'agent', consumer_id = agent_id, binding_instance_id = ''",
		"CREATE TABLE IF NOT EXISTS public.workflow_connector_binding_backfills",
		"workflow_id uuid PRIMARY KEY REFERENCES public.workflows(id) ON DELETE CASCADE",
		"completed_at timestamptz",
		"INSERT INTO public.workflow_connector_binding_backfills (workflow_id) SELECT workflow.id FROM public.workflows AS workflow WHERE lower(workflow.graph) LIKE '%connector%' ON CONFLICT (workflow_id) DO NOTHING",
		"CREATE OR REPLACE FUNCTION public.normalize_agent_resource_binding_consumer()",
		"CREATE TRIGGER agent_resource_bindings_consumer_guard",
		"ALTER COLUMN consumer_id SET NOT NULL",
		"consumer_type = 'agent' AND consumer_id = agent_id AND binding_instance_id = ''",
		"consumer_type = 'workflow' AND btrim(binding_instance_id) <> ''",
		"DROP INDEX IF EXISTS public.idx_agent_resource_bindings_scope_unique",
		"DROP INDEX IF EXISTS public.idx_agent_resource_bindings_integration_unique",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_agent_scope_unique",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_workflow_scope_unique",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_agent_integration_unique",
		"lower(btrim(parent_resource_id))",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_workflow_integration_unique",
		"binding_instance_id ) WHERE consumer_type = 'workflow' AND binding_type = 'integration_connection'",
		"CREATE INDEX IF NOT EXISTS idx_agent_resource_bindings_consumer_scope",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("resource binding consumer migration missing %q: %s", want, sql)
		}
	}
}
