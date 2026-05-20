package calldatabase

import "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"

// DataSourceConfig holds metadata about the selected data source.
type DataSourceConfig struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// TableRef represents a selected table with optional column listing.
type TableRef struct {
	Schema  string   `json:"schema,omitempty"`
	Name    string   `json:"name,omitempty"`
	TableID int      `json:"table_id,omitempty"`
	Columns []string `json:"columns,omitempty"`
}

// ExecutionConfig captures execution related overrides.
type ExecutionConfig struct {
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	MaxRetries     int `json:"max_retries,omitempty"`
}

// NodeData stores configuration for the call-database node.
type NodeData struct {
	base.NodeData

	DataSource     DataSourceConfig `json:"data_source"`
	TableSelection []TableRef       `json:"table_selection"`
	ManualSQL      string           `json:"manual_sql"`
	Execution      ExecutionConfig  `json:"execution"`

	// IsStaticConfig marks whether this config came from graph (true) or runtime (false)
	// This field is not serialized to JSON
	IsStaticConfig bool `json:"-"`
}

// ensureDefaults normalises optional fields.
func (nd *NodeData) ensureDefaults() {
	if nd.Execution.TimeoutSeconds < 0 {
		nd.Execution.TimeoutSeconds = 0
	}
	if nd.Execution.MaxRetries < 0 {
		nd.Execution.MaxRetries = 0
	}
}
