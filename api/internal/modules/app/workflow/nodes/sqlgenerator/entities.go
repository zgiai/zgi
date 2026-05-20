package sqlgenerator

import (
	"encoding/json"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/calldatabase"
)

const AppType = "agent"

// ModelSection captures the model configuration selected by the user.
type ModelSection struct {
	Provider   string         `json:"provider"`
	Name       string         `json:"name"`
	Mode       string         `json:"mode,omitempty"`
	Parameters map[string]any `json:"completion_params,omitempty"`
}

// DataSourceSection keeps the selected data source and tables.
type DataSourceSection struct {
	Source calldatabase.DataSourceConfig `json:"source"`
	Tables []calldatabase.TableRef       `json:"tables"`
}

// VariableSelector describes a variable path selected from the variable pool.
type VariableSelector struct {
	Variable      string   `json:"variable"`
	ValueSelector []string `json:"value_selector"`
}

// MetadataMode indicates how table metadata should be rendered.
type MetadataMode string

const (
	MetadataModeColumns MetadataMode = "columns"
	MetadataModeDDL     MetadataMode = "ddl"
)

// MetadataConfig controls metadata rendering behaviour.
type MetadataConfig struct {
	Mode            MetadataMode `json:"mode,omitempty"`
	MaxColumns      int          `json:"max_columns,omitempty"`
	IncludeComments bool         `json:"include_comments,omitempty"`
}

// PromptSection contains prompt strings and quick variable bindings.
type PromptSection struct {
	System        string             `json:"system"`
	User          string             `json:"user"`
	QuickBindings []VariableSelector `json:"quick_bindings,omitempty"`
	Metadata      MetadataConfig     `json:"metadata"`
}

// ExecutionConfig captures timeout and retry settings for the LLM call.
type ExecutionConfig struct {
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	MaxRetries     int `json:"max_retries,omitempty"`
}

// NodeData represents the SQL generator node configuration.
type NodeData struct {
	base.NodeData

	Model      ModelSection      `json:"model"`
	DataSource DataSourceSection `json:"data_source"`
	Prompt     PromptSection     `json:"prompt"`
	Execution  ExecutionConfig   `json:"execution"`

	// IsStaticConfig marks whether this config came from graph (true) or runtime (false)
	// This field is not serialized to JSON
	IsStaticConfig bool `json:"-"`
}

var defaultMetadataConfig = MetadataConfig{
	Mode:            MetadataModeColumns,
	MaxColumns:      30,
	IncludeComments: false,
}

// ensureDefaults normalises optional fields.
func (nd *NodeData) ensureDefaults() {
	if nd.Model.Parameters == nil {
		nd.Model.Parameters = make(map[string]any)
	}
	if strings.TrimSpace(nd.Model.Mode) == "" {
		nd.Model.Mode = "chat"
	}

	if nd.Execution.TimeoutSeconds <= 0 {
		nd.Execution.TimeoutSeconds = 30
	}
	if nd.Execution.MaxRetries < 0 {
		nd.Execution.MaxRetries = 0
	}

	nd.Prompt.Metadata = defaultMetadataConfig
	nd.Prompt.QuickBindings = deriveVariableSelectorsFromPrompt(nd.Prompt.User)
}

func (ps *PromptSection) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		ps.User = text
		return nil
	}

	type Alias PromptSection
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*ps = PromptSection(aux)
	return nil
}
