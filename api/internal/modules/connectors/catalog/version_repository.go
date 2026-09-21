package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// PluginRuntimeState is the durable current definition set. Definitions for
// inactive plugins are retained so impact and maintenance operations remain
// possible after restart; callers expose only ActiveDefinitions to members.
type PluginRuntimeState struct {
	Definitions []Definition
	Statuses    map[string]PluginDefinitionStatus
}

func (state PluginRuntimeState) ActiveDefinitions() []Definition {
	result := make([]Definition, 0, len(state.Definitions))
	for _, definition := range state.Definitions {
		if state.Statuses[definition.PluginID] == PluginDefinitionStatusActive {
			result = append(result, cloneDefinition(definition))
		}
	}
	return result
}

type AcceptedDefinitionVersion struct {
	Definition     Definition
	ContractDigest string
	Status         PluginDefinitionStatus
	AcceptedAt     time.Time
	ActivatedAt    *time.Time
	RetiredAt      *time.Time
}

func (repository *GormDefinitionRepository) activatePackage(ctx context.Context, definitions []Definition) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("admit Connector definitions: %w", ErrPluginAdmissionUnavailable)
	}
	candidates, pluginID, err := validatePluginPackageDefinitions(definitions)
	if err != nil {
		return err
	}
	ordered := make([]Definition, 0, len(candidates))
	for _, definition := range candidates {
		ordered = append(ordered, definition)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "connector-plugin:"+pluginID); result.Error != nil {
			return fmt.Errorf("lock Connector plugin admission: %w", result.Error)
		}
		currentRows, err := loadStoredDefinitionRows(tx, pluginID, nil)
		if err != nil {
			return err
		}
		current := make(map[string]Definition, len(currentRows))
		for _, row := range currentRows {
			if row.Status != string(PluginDefinitionStatusActive) {
				return ErrPluginDefinitionInactive
			}
			definition, decodeErr := row.definition()
			if decodeErr != nil {
				return decodeErr
			}
			current[definition.ID] = definition
		}
		if err := validateActivePackageReplacement(current, candidates, pluginID); err != nil {
			return err
		}
		// An exact package replay is already durably admitted and must not
		// manufacture a second lifecycle event during process bootstrap.
		if sameStoredPluginPackage(current, candidates) {
			return nil
		}
		ids := make([]string, 0, len(ordered))
		for _, definition := range ordered {
			ids = append(ids, definition.ID)
		}
		var owners []struct {
			ConnectorID string `gorm:"column:connector_id"`
			PluginID    string `gorm:"column:plugin_id"`
		}
		if err := tx.Table("connector_definitions").Select("connector_id, plugin_id").
			Where("connector_id IN ?", ids).Scan(&owners).Error; err != nil {
			return fmt.Errorf("check Connector identity ownership: %w", err)
		}
		for _, owner := range owners {
			if owner.PluginID != pluginID {
				return fmt.Errorf("%w: %s", ErrDuplicateConnector, owner.ConnectorID)
			}
		}

		now := time.Now().UTC()
		firstRecord, err := definitionAdmissionRecord(ordered[0], now)
		if err != nil {
			return err
		}
		if result := tx.Table("connector_plugin_versions").
			Where("plugin_id = ? AND status = ? AND NOT (plugin_version = ? AND plugin_digest = ?)",
				pluginID, PluginDefinitionStatusActive, firstRecord.PluginVersion, firstRecord.PluginDigest).
			Updates(map[string]interface{}{
				"status": PluginDefinitionStatusSuperseded, "retired_at": now, "updated_at": now,
			}); result.Error != nil {
			return fmt.Errorf("supersede prior Connector plugin version: %w", result.Error)
		}
		if result := tx.Table("connector_definition_versions").
			Where("plugin_id = ? AND status = ? AND NOT (plugin_version = ? AND plugin_digest = ?)",
				pluginID, PluginDefinitionStatusActive, firstRecord.PluginVersion, firstRecord.PluginDigest).
			Updates(map[string]interface{}{
				"status": PluginDefinitionStatusSuperseded, "retired_at": now, "updated_at": now,
			}); result.Error != nil {
			return fmt.Errorf("supersede prior Connector definition versions: %w", result.Error)
		}
		pluginResult := tx.Exec(`
			INSERT INTO connector_plugin_versions (
				plugin_id, plugin_version, plugin_digest, source_dialect, schema_version,
				status, accepted_at, activated_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)
			ON CONFLICT (plugin_id, plugin_version, plugin_digest) DO UPDATE SET
				status = 'active', activated_at = EXCLUDED.activated_at,
				retired_at = NULL, updated_at = EXCLUDED.updated_at
			WHERE connector_plugin_versions.source_dialect = EXCLUDED.source_dialect
				AND connector_plugin_versions.schema_version = EXCLUDED.schema_version`,
			firstRecord.PluginID, firstRecord.PluginVersion, firstRecord.PluginDigest,
			firstRecord.SourceDialect, firstRecord.SchemaVersion, now, now, now, now,
		)
		if pluginResult.Error != nil {
			return fmt.Errorf("persist accepted Connector plugin version: %w", pluginResult.Error)
		}
		if pluginResult.RowsAffected != 1 {
			return fmt.Errorf("%w: %s", ErrIncompatibleStoredDefinition, pluginID)
		}

		for _, definition := range ordered {
			record, recordErr := definitionAdmissionRecord(definition, now)
			if recordErr != nil {
				return recordErr
			}
			encoded, encodeErr := json.Marshal(definition)
			if encodeErr != nil {
				return fmt.Errorf("encode accepted Connector definition %s: %w", definition.ID, encodeErr)
			}
			versionResult := tx.Exec(`
				INSERT INTO connector_definition_versions (
					connector_id, plugin_id, plugin_version, plugin_digest, contract_digest,
					definition, status, accepted_at, activated_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, CAST(? AS jsonb), 'active', ?, ?, ?, ?)
				ON CONFLICT (connector_id, plugin_version, plugin_digest) DO UPDATE SET
					status = 'active', activated_at = EXCLUDED.activated_at,
					retired_at = NULL, updated_at = EXCLUDED.updated_at
				WHERE connector_definition_versions.plugin_id = EXCLUDED.plugin_id
					AND connector_definition_versions.contract_digest = EXCLUDED.contract_digest
					AND connector_definition_versions.definition = EXCLUDED.definition`,
				record.ConnectorID, record.PluginID, record.PluginVersion, record.PluginDigest,
				record.ContractDigest, string(encoded), now, now, now, now,
			)
			if versionResult.Error != nil {
				return fmt.Errorf("persist accepted Connector definition version %s: %w", definition.ID, versionResult.Error)
			}
			if versionResult.RowsAffected != 1 {
				return fmt.Errorf("%w: %s", ErrIncompatibleStoredDefinition, definition.ID)
			}
			if prior, exists := current[definition.ID]; exists && prior.Classification != definition.Classification {
				// A reviewed correction changes only the current projection. The
				// old accepted definition and contract digest remain immutable;
				// the ordinary upsert below keeps its classification equality guard.
				if !reviewedClassificationCorrection(prior, definition) {
					return incompatible("classification", "")
				}
				priorDigest, digestErr := DefinitionContractDigest(prior)
				if digestErr != nil {
					return digestErr
				}
				correction := tx.Table("connector_definitions").
					Where("connector_id = ? AND plugin_id = ? AND plugin_version = ? AND plugin_digest = ? AND contract_digest = ? AND classification = ? AND status = ?",
						prior.ID, prior.PluginID, prior.PluginVersion, prior.PluginDigest, priorDigest, prior.Classification, PluginDefinitionStatusActive).
					Updates(map[string]interface{}{"classification": record.Classification, "contract_digest": record.ContractDigest})
				if correction.Error != nil {
					return fmt.Errorf("correct current Connector classification %s: %w", definition.ID, correction.Error)
				}
				if correction.RowsAffected != 1 {
					return fmt.Errorf("%w: %s classification correction", ErrIncompatibleStoredDefinition, definition.ID)
				}
			}
			currentResult := tx.Exec(`
				INSERT INTO connector_definitions (
					connector_id, plugin_id, plugin_version, plugin_digest,
					source_dialect, schema_version, provider_id, driver_id,
					classification, display, key_schema, capability_summary,
					contract_digest, status, accepted_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb), CAST(? AS jsonb), CAST(? AS jsonb), ?, 'active', ?, ?, ?)
				ON CONFLICT (connector_id) DO UPDATE SET
					plugin_version = EXCLUDED.plugin_version,
					plugin_digest = EXCLUDED.plugin_digest,
					display = EXCLUDED.display,
					key_schema = EXCLUDED.key_schema,
					capability_summary = EXCLUDED.capability_summary,
					contract_digest = EXCLUDED.contract_digest,
					status = 'active', accepted_at = EXCLUDED.accepted_at,
					updated_at = EXCLUDED.updated_at
				WHERE connector_definitions.plugin_id = EXCLUDED.plugin_id
					AND connector_definitions.source_dialect = EXCLUDED.source_dialect
					AND connector_definitions.schema_version = EXCLUDED.schema_version
					AND connector_definitions.provider_id = EXCLUDED.provider_id
					AND connector_definitions.driver_id = EXCLUDED.driver_id
					AND connector_definitions.classification = EXCLUDED.classification`,
				record.ConnectorID, record.PluginID, record.PluginVersion, record.PluginDigest,
				record.SourceDialect, record.SchemaVersion, record.ProviderID, record.DriverID,
				record.Classification, record.Display, record.KeySchema, record.CapabilitySummary,
				record.ContractDigest, record.AcceptedAt, record.CreatedAt, record.UpdatedAt,
			)
			if currentResult.Error != nil {
				return fmt.Errorf("activate Connector definition %s: %w", definition.ID, currentResult.Error)
			}
			if currentResult.RowsAffected != 1 {
				return fmt.Errorf("%w: %s", ErrIncompatibleStoredDefinition, definition.ID)
			}
		}
		return recordPluginAdmissionAudit(ctx, tx, pluginID, ordered, current, now)
	})
}

func (repository *GormDefinitionRepository) LoadRuntimeState(ctx context.Context) (PluginRuntimeState, error) {
	if repository == nil || repository.db == nil {
		return PluginRuntimeState{}, ErrPluginAdmissionUnavailable
	}
	rows, err := loadStoredDefinitionRows(repository.db.WithContext(ctx), "", nil)
	if err != nil {
		return PluginRuntimeState{}, err
	}
	state := PluginRuntimeState{Statuses: make(map[string]PluginDefinitionStatus)}
	for _, row := range rows {
		definition, decodeErr := row.definition()
		if decodeErr != nil {
			return PluginRuntimeState{}, decodeErr
		}
		status := PluginDefinitionStatus(row.Status)
		if status != PluginDefinitionStatusActive && status != PluginDefinitionStatusDisabled && status != PluginDefinitionStatusUnavailable {
			return PluginRuntimeState{}, fmt.Errorf("stored Connector plugin status %q is invalid", status)
		}
		if prior, exists := state.Statuses[definition.PluginID]; exists && prior != status {
			return PluginRuntimeState{}, fmt.Errorf("stored Connector plugin %s has inconsistent status", definition.PluginID)
		}
		state.Statuses[definition.PluginID] = status
		state.Definitions = append(state.Definitions, definition)
	}
	return state, nil
}

func (repository *GormDefinitionRepository) ListAcceptedVersions(
	ctx context.Context,
	pluginID string,
) ([]AcceptedDefinitionVersion, error) {
	if repository == nil || repository.db == nil || !stableIdentifier(pluginID) {
		return nil, ErrPluginDefinitionNotFound
	}
	type versionRow struct {
		ConnectorID    string     `gorm:"column:connector_id"`
		PluginID       string     `gorm:"column:plugin_id"`
		PluginVersion  string     `gorm:"column:plugin_version"`
		PluginDigest   string     `gorm:"column:plugin_digest"`
		ContractDigest string     `gorm:"column:contract_digest"`
		Definition     string     `gorm:"column:definition"`
		Status         string     `gorm:"column:status"`
		AcceptedAt     time.Time  `gorm:"column:accepted_at"`
		ActivatedAt    *time.Time `gorm:"column:activated_at"`
		RetiredAt      *time.Time `gorm:"column:retired_at"`
	}
	var rows []versionRow
	if err := repository.db.WithContext(ctx).Table("connector_definition_versions").
		Select("connector_id, plugin_id, plugin_version, plugin_digest, contract_digest, CAST(definition AS text) AS definition, status, accepted_at, activated_at, retired_at").
		Where("plugin_id = ?", pluginID).
		Order("accepted_at DESC, connector_id ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list accepted Connector definition versions: %w", err)
	}
	result := make([]AcceptedDefinitionVersion, 0, len(rows))
	for _, row := range rows {
		var definition Definition
		if err := json.Unmarshal([]byte(row.Definition), &definition); err != nil {
			return nil, fmt.Errorf("decode accepted Connector definition %s: %w", row.ConnectorID, err)
		}
		if definition.ID != row.ConnectorID || definition.PluginID != row.PluginID ||
			definition.PluginVersion != row.PluginVersion || !sha256HexPattern.MatchString(row.PluginDigest) ||
			!sha256HexPattern.MatchString(row.ContractDigest) {
			return nil, fmt.Errorf("%w: %s accepted version identity", ErrIncompatibleStoredDefinition, row.ConnectorID)
		}
		definition.PluginDigest = row.PluginDigest
		if err := Validate(definition); err != nil {
			return nil, fmt.Errorf("%w: %s accepted version: %w", ErrIncompatibleStoredDefinition, row.ConnectorID, err)
		}
		contractDigest, err := DefinitionContractDigest(definition)
		if err != nil || !strings.EqualFold(contractDigest, row.ContractDigest) {
			return nil, fmt.Errorf("%w: %s accepted version contract digest", ErrIncompatibleStoredDefinition, row.ConnectorID)
		}
		status := PluginDefinitionStatus(row.Status)
		if status != PluginDefinitionStatusActive && status != PluginDefinitionStatusSuperseded &&
			status != PluginDefinitionStatusDisabled && status != PluginDefinitionStatusUnavailable {
			return nil, fmt.Errorf("%w: %s accepted version status", ErrIncompatibleStoredDefinition, row.ConnectorID)
		}
		result = append(result, AcceptedDefinitionVersion{
			Definition: definition, ContractDigest: row.ContractDigest,
			Status: status, AcceptedAt: row.AcceptedAt,
			ActivatedAt: row.ActivatedAt, RetiredAt: row.RetiredAt,
		})
	}
	return result, nil
}

type storedDefinitionRow struct {
	ConnectorID       string `gorm:"column:connector_id"`
	PluginID          string `gorm:"column:plugin_id"`
	PluginVersion     string `gorm:"column:plugin_version"`
	PluginDigest      string `gorm:"column:plugin_digest"`
	SourceDialect     string `gorm:"column:source_dialect"`
	SchemaVersion     string `gorm:"column:schema_version"`
	ProviderID        string `gorm:"column:provider_id"`
	DriverID          string `gorm:"column:driver_id"`
	Classification    string `gorm:"column:classification"`
	Display           string `gorm:"column:display"`
	KeySchema         string `gorm:"column:key_schema"`
	CapabilitySummary string `gorm:"column:capability_summary"`
	ContractDigest    string `gorm:"column:contract_digest"`
	Status            string `gorm:"column:status"`
}

func loadStoredDefinitionRows(db *gorm.DB, pluginID string, connectorIDs []string) ([]storedDefinitionRow, error) {
	query := db.Table("connector_definitions").Select(
		"connector_id, plugin_id, plugin_version, plugin_digest, source_dialect, schema_version, provider_id, driver_id, classification, " +
			"CAST(display AS text) AS display, CAST(key_schema AS text) AS key_schema, CAST(capability_summary AS text) AS capability_summary, contract_digest, status",
	)
	if pluginID != "" {
		query = query.Where("plugin_id = ?", pluginID)
	}
	if len(connectorIDs) > 0 {
		query = query.Where("connector_id IN ?", connectorIDs)
	}
	var rows []storedDefinitionRow
	if err := query.Order("connector_id ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load current Connector definitions: %w", err)
	}
	return rows, nil
}

func (row storedDefinitionRow) definition() (Definition, error) {
	if !sha256HexPattern.MatchString(row.PluginDigest) || !sha256HexPattern.MatchString(row.ContractDigest) {
		return Definition{}, fmt.Errorf("%w: %s stored digest", ErrIncompatibleStoredDefinition, row.ConnectorID)
	}
	var display struct {
		Name            string        `json:"name"`
		NameI18n        LocalizedText `json:"name_i18n"`
		Description     string        `json:"description"`
		DescriptionI18n LocalizedText `json:"description_i18n"`
		Icon            string        `json:"icon"`
	}
	var keySchema struct {
		SetupMethod            SetupMethod       `json:"setup_method"`
		Ownership              Ownership         `json:"ownership"`
		AuthMethodID           string            `json:"auth_method_id"`
		Fields                 []KeyField        `json:"fields"`
		ExtensionSchemaVersion string            `json:"extension_schema_version"`
		ContractVersion        int               `json:"contract_version"`
		CredentialFields       []CredentialField `json:"credential_fields"`
		Setup                  *SetupDescriptor  `json:"setup"`
	}
	var capabilities struct {
		Types      []CapabilityType `json:"types"`
		Surfaces   []Surface        `json:"surfaces"`
		Operations []Operation      `json:"operations"`
	}
	if err := json.Unmarshal([]byte(row.Display), &display); err != nil {
		return Definition{}, fmt.Errorf("decode Connector definition %s display: %w", row.ConnectorID, err)
	}
	if err := json.Unmarshal([]byte(row.KeySchema), &keySchema); err != nil {
		return Definition{}, fmt.Errorf("decode Connector definition %s key schema: %w", row.ConnectorID, err)
	}
	if err := json.Unmarshal([]byte(row.CapabilitySummary), &capabilities); err != nil {
		return Definition{}, fmt.Errorf("decode Connector definition %s capabilities: %w", row.ConnectorID, err)
	}
	definition := Definition{
		ID: row.ConnectorID, PluginID: row.PluginID, PluginVersion: row.PluginVersion,
		PluginDigest: row.PluginDigest, SourceDialect: row.SourceDialect,
		SourceSchemaVersion: row.SchemaVersion, ProviderID: row.ProviderID, DriverID: row.DriverID,
		AuthMethodID: keySchema.AuthMethodID, Name: display.Name, Description: display.Description, Icon: display.Icon,
		NameI18n: display.NameI18n, DescriptionI18n: display.DescriptionI18n,
		Classification: Classification(row.Classification), CapabilityTypes: capabilities.Types,
		SetupMethod: keySchema.SetupMethod, Ownership: keySchema.Ownership,
		Surfaces: capabilities.Surfaces, KeyFields: keySchema.Fields, Operations: capabilities.Operations,
		ExtensionSchemaVersion: keySchema.ExtensionSchemaVersion, ContractVersion: keySchema.ContractVersion,
		CredentialFields: keySchema.CredentialFields, Setup: keySchema.Setup,
	}
	if err := Validate(definition); err != nil {
		return Definition{}, fmt.Errorf("%w: %s: %w", ErrIncompatibleStoredDefinition, row.ConnectorID, err)
	}
	digest, err := DefinitionContractDigest(definition)
	if err != nil || !strings.EqualFold(digest, row.ContractDigest) {
		return Definition{}, fmt.Errorf("%w: %s contract digest", ErrIncompatibleStoredDefinition, row.ConnectorID)
	}
	return definition, nil
}
