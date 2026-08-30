package sqlgenerator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/calldatabase"
)

func TestNodeDataTableSelectionModeJSON(t *testing.T) {
	var automatic NodeData
	if err := json.Unmarshal([]byte(`{"table_selection_mode":"automatic"}`), &automatic); err != nil {
		t.Fatalf("unmarshal automatic table selection mode: %v", err)
	}
	if automatic.TableSelectionMode != TableSelectionModeAutomatic {
		t.Fatalf("table selection mode = %q, want %q", automatic.TableSelectionMode, TableSelectionModeAutomatic)
	}

	var invalid NodeData
	err := json.Unmarshal([]byte(`{"table_selection_mode":"future-mode"}`), &invalid)
	if err == nil || !strings.Contains(err.Error(), "table_selection_mode") {
		t.Fatalf("unmarshal invalid table selection mode error = %v", err)
	}
}

func TestNodeDataRejectsStopParameter(t *testing.T) {
	data := NodeData{
		Model:      ModelSection{Provider: "provider", Name: "model", Parameters: map[string]any{"stop": []string{"END"}}},
		Prompt:     PromptSection{User: "question"},
		DataSource: DataSourceSection{Source: calldatabase.DataSourceConfig{ID: "ds"}, Tables: []calldatabase.TableRef{{TableID: 7}}},
	}
	if err := data.ValidateConfiguration(); err == nil || !strings.Contains(err.Error(), "completion_params.stop") {
		t.Fatalf("ValidateConfiguration error = %v", err)
	}
}

func TestNodeDataRejectsCaseInsensitiveStopParameter(t *testing.T) {
	data := NodeData{
		Model:      ModelSection{Provider: "provider", Name: "model", Parameters: map[string]any{"STOP": "END"}},
		Prompt:     PromptSection{User: "question"},
		DataSource: DataSourceSection{Source: calldatabase.DataSourceConfig{ID: "ds"}, Tables: []calldatabase.TableRef{{TableID: 7}}},
	}
	if err := data.ValidateConfiguration(); err == nil || !strings.Contains(err.Error(), "completion_params.stop") {
		t.Fatalf("ValidateConfiguration error = %v", err)
	}
}

func TestNodeDataDefaultsPreserveMetadataConfiguration(t *testing.T) {
	data := NodeData{Prompt: PromptSection{Metadata: MetadataConfig{Mode: MetadataModeDDL, MaxColumns: 12, IncludeComments: true}}}
	data.ensureDefaults()
	if data.Prompt.Metadata.Mode != MetadataModeDDL || data.Prompt.Metadata.MaxColumns != 12 || !data.Prompt.Metadata.IncludeComments {
		t.Fatalf("metadata defaults overwrote configuration: %#v", data.Prompt.Metadata)
	}
}
