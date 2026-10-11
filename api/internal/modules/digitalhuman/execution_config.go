package digitalhuman

import (
	"bytes"
	"encoding/json"

	"gorm.io/datatypes"
)

// ExecutionRuntimeConfig is the application's opt-in, not an environment policy.
// Backend, network, dependency installation and snapshots remain operator-owned.
type ExecutionRuntimeConfig struct {
	Enabled bool `json:"enabled"`
}

func (c *ExecutionRuntimeConfig) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 1 || fields["enabled"] == nil || bytes.Equal(bytes.TrimSpace(fields["enabled"]), []byte("null")) {
		return ErrInvalidRequest
	}
	var enabled bool
	if json.Unmarshal(fields["enabled"], &enabled) != nil {
		return ErrInvalidRequest
	}
	c.Enabled = enabled
	return nil
}

// Disabled settings normalize to absence so old revisions and persisted v1
// snapshot digests remain byte-for-byte compatible with the previous schema.
func cloneExecutionRuntime(config *ExecutionRuntimeConfig) *ExecutionRuntimeConfig {
	if config == nil || !config.Enabled {
		return nil
	}
	return &ExecutionRuntimeConfig{Enabled: true}
}

func executionRuntimeJSON(config *ExecutionRuntimeConfig) datatypes.JSON {
	if config == nil || !config.Enabled {
		return nil
	}
	return datatypes.JSON(`{"enabled":true}`)
}
