package database

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type databaseBindingSet struct {
	Readable map[string]struct{}
	Writable map[string]struct{}
}

type databaseBindings map[string]databaseBindingSet

func (t *databaseTool) agentBindings() (databaseBindings, error) {
	runtime := t.Runtime()
	if runtime == nil || runtime.InvokeFrom != tools.ToolInvokeFromAgent {
		return nil, nil
	}
	raw, ok := runtime.RuntimeParameters["database_bindings"]
	if !ok || raw == nil {
		return databaseBindings{}, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid database bindings: %w", err)
	}
	var parsed []struct {
		DataSourceID     string   `json:"data_source_id"`
		TableIDs         []string `json:"table_ids"`
		WritableTableIDs []string `json:"writable_table_ids"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("invalid database bindings: %w", err)
	}
	out := databaseBindings{}
	for _, binding := range parsed {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		if _, ok := out[dataSourceID]; !ok {
			out[dataSourceID] = databaseBindingSet{Readable: map[string]struct{}{}, Writable: map[string]struct{}{}}
		}
		set := out[dataSourceID]
		for _, rawTableID := range binding.TableIDs {
			tableID := strings.TrimSpace(rawTableID)
			if tableID != "" {
				set.Readable[tableID] = struct{}{}
			}
		}
		for _, rawTableID := range binding.WritableTableIDs {
			tableID := strings.TrimSpace(rawTableID)
			if tableID != "" {
				if _, ok := set.Readable[tableID]; ok {
					set.Writable[tableID] = struct{}{}
				}
			}
		}
		out[dataSourceID] = set
	}
	return out, nil
}

func (b databaseBindings) dataSourceAllowed(dataSourceID string) bool {
	if b == nil {
		return true
	}
	set, ok := b[strings.TrimSpace(dataSourceID)]
	return ok && len(set.Readable) > 0
}

func (b databaseBindings) tableAllowed(dataSourceID string, tableID string) bool {
	if b == nil {
		return true
	}
	set, ok := b[strings.TrimSpace(dataSourceID)]
	if !ok {
		return false
	}
	_, ok = set.Readable[strings.TrimSpace(tableID)]
	return ok
}

func (b databaseBindings) tableWritable(dataSourceID string, tableID string) bool {
	if b == nil {
		return true
	}
	set, ok := b[strings.TrimSpace(dataSourceID)]
	if !ok {
		return false
	}
	_, ok = set.Writable[strings.TrimSpace(tableID)]
	return ok
}

func (b databaseBindings) dataSourceIDs() []string {
	ids := make([]string, 0, len(b))
	for id, set := range b {
		if strings.TrimSpace(id) != "" && len(set.Readable) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
