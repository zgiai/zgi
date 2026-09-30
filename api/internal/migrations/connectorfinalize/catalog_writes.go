package connectorfinalize

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"gorm.io/gorm"
)

type catalogChange struct {
	Before         row
	After          catalog.Definition
	ContractDigest string
}

func catalogChanges(data snapshot) ([]catalogChange, error) {
	changes := []catalogChange{}
	plugins := map[string]bool{}
	for _, current := range data["connector_definitions"] {
		plugin := text(current, "plugin_id")
		if plugins[plugin] {
			return nil, fmt.Errorf("multi-definition package requires its complete maintenance manifest")
		}
		plugins[plugin] = true
		var accepted row
		for _, version := range data["connector_definition_versions"] {
			if text(version, "connector_id") == text(current, "connector_id") && text(version, "plugin_version") == text(current, "plugin_version") && text(version, "plugin_digest") == text(current, "plugin_digest") {
				accepted = version
				break
			}
		}
		if accepted == nil {
			return nil, fmt.Errorf("current accepted definition required")
		}
		var before catalog.Definition
		encoded, err := json.Marshal(accepted["definition"])
		if err != nil || json.Unmarshal(encoded, &before) != nil {
			return nil, fmt.Errorf("accepted definition cannot be decoded")
		}
		before.PluginDigest = text(current, "plugin_digest")
		after, err := PlanCatalogTransition(before)
		if err != nil {
			return nil, err
		}
		if after.PluginVersion == before.PluginVersion {
			continue
		}
		for _, version := range data["connector_definition_versions"] {
			if text(version, "connector_id") == after.ID && text(version, "plugin_version") == after.PluginVersion {
				return nil, fmt.Errorf("maintenance definition version already occupied")
			}
		}
		for _, version := range data["connector_plugin_versions"] {
			if text(version, "plugin_id") == after.PluginID && text(version, "plugin_version") == after.PluginVersion {
				return nil, fmt.Errorf("maintenance plugin version already occupied")
			}
		}
		commitment, err := catalog.DefinitionContractDigest(after)
		if err != nil {
			return nil, err
		}
		changes = append(changes, catalogChange{Before: current, After: after, ContractDigest: commitment})
	}
	return changes, nil
}

// applyCatalogChanges is called only inside the locked retirement transaction,
// after its input snapshot commitment has been rechecked. Ordinary plugin
// admission never calls this SkillID-only maintenance path.
func applyCatalogChanges(tx *gorm.DB, changes []catalogChange, now time.Time) error {
	plugins := map[string]bool{}
	for _, change := range changes {
		d := change.After
		status := text(change.Before, "status")
		if !plugins[d.PluginID] {
			retired := tx.Table("connector_plugin_versions").Where("plugin_id=? AND plugin_version=? AND plugin_digest=?", d.PluginID, text(change.Before, "plugin_version"), text(change.Before, "plugin_digest")).Updates(map[string]any{"status": "superseded", "retired_at": now, "updated_at": now})
			if retired.Error != nil || retired.RowsAffected != 1 {
				return fmt.Errorf("retire prior accepted plugin version")
			}
			insert := tx.Table("connector_plugin_versions").Create(map[string]any{"plugin_id": d.PluginID, "plugin_version": d.PluginVersion, "plugin_digest": d.PluginDigest, "source_dialect": d.SourceDialect, "schema_version": d.SourceSchemaVersion, "status": status, "accepted_at": now, "activated_at": now, "created_at": now, "updated_at": now})
			if insert.Error != nil || insert.RowsAffected != 1 {
				return fmt.Errorf("insert maintenance plugin version")
			}
			plugins[d.PluginID] = true
		}
		retired := tx.Table("connector_definition_versions").Where("connector_id=? AND plugin_version=? AND plugin_digest=? AND contract_digest=?", d.ID, text(change.Before, "plugin_version"), text(change.Before, "plugin_digest"), text(change.Before, "contract_digest")).Updates(map[string]any{"status": "superseded", "retired_at": now, "updated_at": now})
		if retired.Error != nil || retired.RowsAffected != 1 {
			return fmt.Errorf("retire prior accepted definition version")
		}
		encoded, err := json.Marshal(d)
		if err != nil {
			return err
		}
		insert := tx.Table("connector_definition_versions").Create(map[string]any{"connector_id": d.ID, "plugin_id": d.PluginID, "plugin_version": d.PluginVersion, "plugin_digest": d.PluginDigest, "contract_digest": change.ContractDigest, "definition": gorm.Expr("?::jsonb", string(encoded)), "status": status, "accepted_at": now, "activated_at": now, "created_at": now, "updated_at": now})
		if insert.Error != nil || insert.RowsAffected != 1 {
			return fmt.Errorf("insert maintenance definition version")
		}
		capabilities, err := json.Marshal(map[string]any{"types": d.CapabilityTypes, "surfaces": d.Surfaces, "operations": d.Operations})
		if err != nil {
			return err
		}
		updated := tx.Table("connector_definitions").Where("connector_id=? AND plugin_version=? AND plugin_digest=? AND contract_digest=?", d.ID, text(change.Before, "plugin_version"), text(change.Before, "plugin_digest"), text(change.Before, "contract_digest")).Updates(map[string]any{"plugin_version": d.PluginVersion, "plugin_digest": d.PluginDigest, "contract_digest": change.ContractDigest, "capability_summary": gorm.Expr("?::jsonb", string(capabilities)), "accepted_at": now, "updated_at": now})
		if updated.Error != nil || updated.RowsAffected != 1 {
			return fmt.Errorf("replace current definition projection")
		}
	}
	return nil
}
