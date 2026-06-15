package agents

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentPromptEffectiveLengthIncludesDatabaseAndTableBlocks(t *testing.T) {
	source := `A <zgi:database id="db-1">Orders DB</zgi:database> B <zgi:table id="db-1:tbl-1">Orders</zgi:table> C <zgi:workflow id="wf-1">Refund Flow</zgi:workflow>`

	if got, want := agentPromptEffectiveLength(source), len([]rune("A Orders DB B Orders C Refund Flow")); got != want {
		t.Fatalf("agentPromptEffectiveLength() = %d, want %d", got, want)
	}
}

func TestRenderAgentPromptDatabaseIncludesBoundTables(t *testing.T) {
	got := renderAgentPromptDatabase(agentPromptDatabaseSummary{
		ID:          "db-1",
		Name:        "Operations",
		SchemaName:  "ops",
		Description: "Operational records",
		Tables: []agentPromptTableSummary{
			{ID: "table-1", Name: "Customers", Description: "Customer profile records"},
			{ID: "table-2", Name: "Tickets", Description: "Support ticket records", Writable: true},
		},
	})

	for _, want := range []string{
		"Database: Operations",
		"Schema: ops",
		"Description: Operational records",
		"- Customers: Customer profile records",
		"- Tickets: Support ticket records (writable)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered database summary missing %q:\n%s", want, got)
		}
	}
}

func TestAgentPromptDatabaseSummaryScansWithoutGormRelationError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE data_sources (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		schema_name TEXT
	)`).Error; err != nil {
		t.Fatalf("create data_sources: %v", err)
	}
	if err := db.Exec(`INSERT INTO data_sources (id, name, description, schema_name) VALUES (?, ?, ?, ?)`, "db-1", "Operations", "Operational records", "ops").Error; err != nil {
		t.Fatalf("insert data source: %v", err)
	}

	var rows []agentPromptDatabaseSummary
	err = db.Table("data_sources").
		Select("id, name, COALESCE(description, '') AS description, COALESCE(schema_name, '') AS schema_name").
		Find(&rows).Error
	if err != nil {
		t.Fatalf("scan database summary: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "db-1" || rows[0].Name != "Operations" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestRenderAgentPromptTableVariableSupportsDatabaseScopedKey(t *testing.T) {
	got := renderAgentPromptTableVariable("db-1:table-1", map[string]agentPromptDatabaseSummary{
		"db-1": {
			ID:         "db-1",
			Name:       "Operations",
			SchemaName: "ops",
			Tables: []agentPromptTableSummary{
				{ID: "table-1", Name: "Customers", Description: "Customer profile records"},
			},
		},
	})

	for _, want := range []string{
		"Data table: Customers",
		"Database: Operations",
		"Schema: ops",
		"Description: Customer profile records",
		"Write access: disabled",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered table summary missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAgentPromptWorkflowVariableIncludesBoundWorkflowSummary(t *testing.T) {
	got := renderAgentPromptWorkflowVariable("binding-1", map[string]dto.AgentWorkflowBinding{
		"binding-1": {
			BindingID:       "binding-1",
			Label:           "Refund Review",
			Description:     "Routes refund requests through approval and fulfillment.",
			AgentType:       "WORKFLOW",
			VersionStrategy: "latest_published",
			DefaultInputKey: "query",
			StartInputs: []dto.AgentWorkflowStartInput{
				{Variable: "query", Label: "Customer request", Type: "string", Required: true},
				{Variable: "amount", Label: "Refund amount", Type: "number"},
			},
		},
	})

	for _, want := range []string{
		"Bound workflow: Refund Review",
		"Binding ID: binding-1",
		"Workflow type: WORKFLOW",
		"Description: Routes refund requests through approval and fulfillment.",
		"Version: latest_published",
		"Default input: query",
		"Required inputs: query",
		"- query - Customer request (string) [required]",
		"- amount - Refund amount (number)",
		"Call this workflow through the agent-workflow skill with this binding_id.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered workflow summary missing %q:\n%s", want, got)
		}
	}
}

func TestNormalizeAgentPromptDatabaseBindings(t *testing.T) {
	dataSources, tables, writable := normalizeAgentPromptDatabaseBindings([]dto.AgentDatabaseBinding{
		{
			DataSourceID:     "db-1",
			TableIDs:         []string{"table-1", "table-2", ""},
			WritableTableIDs: []string{"table-2", "not-bound"},
		},
		{
			DataSourceID:     "db-1",
			TableIDs:         []string{"table-3"},
			WritableTableIDs: []string{"table-3"},
		},
	})

	if len(dataSources) != 1 || dataSources[0] != "db-1" {
		t.Fatalf("dataSources = %#v, want [db-1]", dataSources)
	}
	if got := tables["table-1"]; got != "db-1" {
		t.Fatalf("tables[table-1] = %q, want db-1", got)
	}
	if writable["db-1:not-bound"] {
		t.Fatalf("unbound writable table leaked into writable map")
	}
	if !writable["db-1:table-2"] || !writable["db-1:table-3"] {
		t.Fatalf("expected bound writable tables, got %#v", writable)
	}
}
