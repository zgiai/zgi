package agents

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestAgentPromptEffectiveLengthIncludesDatabaseAndTableBlocks(t *testing.T) {
	source := `A <zgi:database id="db-1">Orders DB</zgi:database> B <zgi:table id="db-1:tbl-1">Orders</zgi:table>`

	if got, want := agentPromptEffectiveLength(source), len([]rune("A Orders DB B Orders")); got != want {
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
