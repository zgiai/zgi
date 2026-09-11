package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrIncompatibleStoredDefinition = errors.New("stored Connector definition is incompatible")

type DefinitionAdmitter interface {
	Admit(context.Context, []Definition) error
}

// GormDefinitionRepository admits trusted Connector definitions into the
// authoritative schema before a member can create a setup attempt. An existing
// identity is accepted only when its frozen contract digest still matches;
// plugin lifecycle updates are handled separately and cannot be smuggled in by
// ordinary service startup.
type GormDefinitionRepository struct {
	db *gorm.DB
}

func NewGormDefinitionRepository(db *gorm.DB) *GormDefinitionRepository {
	return &GormDefinitionRepository{db: db}
}

func (repository *GormDefinitionRepository) Admit(ctx context.Context, definitions []Definition) error {
	return repository.activatePackage(ctx, definitions)
}

type definitionRecord struct {
	ConnectorID       string
	PluginID          string
	PluginVersion     string
	PluginDigest      string
	SourceDialect     string
	SchemaVersion     string
	ProviderID        string
	DriverID          string
	Classification    string
	Display           string
	KeySchema         string
	CapabilitySummary string
	ContractDigest    string
	AcceptedAt        time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func definitionAdmissionRecord(definition Definition, now time.Time) (definitionRecord, error) {
	display, err := json.Marshal(map[string]any{
		"name": definition.Name, "description": definition.Description, "icon": definition.Icon,
		"name_i18n": definition.NameI18n, "description_i18n": definition.DescriptionI18n,
	})
	if err != nil {
		return definitionRecord{}, err
	}
	keyDocument := map[string]any{
		"setup_method": definition.SetupMethod, "ownership": definition.Ownership,
		"auth_method_id": definition.AuthMethodID, "fields": definition.KeyFields,
	}
	// Version metadata also belongs to Key-based V4 contracts.
	if definition.ExtensionSchemaVersion != "" {
		keyDocument["extension_schema_version"] = definition.ExtensionSchemaVersion
	}
	if definition.ContractVersion != 0 {
		keyDocument["contract_version"] = definition.ContractVersion
	}
	if definition.UsesCredentials() {
		keyDocument["credential_fields"] = definition.CredentialFields
		keyDocument["setup"] = definition.Setup
	}
	keySchema, err := json.Marshal(keyDocument)
	if err != nil {
		return definitionRecord{}, err
	}
	capabilities, err := json.Marshal(map[string]any{
		"types": definition.CapabilityTypes, "surfaces": definition.Surfaces, "operations": definition.Operations,
	})
	if err != nil {
		return definitionRecord{}, err
	}
	pluginDigest := definition.PluginDigest
	if pluginDigest == "" {
		// Built-in compatibility definitions predate package-content digests.
		// Keep their deterministic identity digest until they are replaced by
		// an admitted Agent Plugin manifest.
		pluginDigest, err = digestJSON(struct {
			PluginID      string `json:"plugin_id"`
			PluginVersion string `json:"plugin_version"`
			Dialect       string `json:"source_dialect"`
			SchemaVersion string `json:"schema_version"`
		}{definition.PluginID, definition.PluginVersion, definition.SourceDialect, definition.SourceSchemaVersion})
		if err != nil {
			return definitionRecord{}, err
		}
	} else if !sha256HexPattern.MatchString(pluginDigest) {
		return definitionRecord{}, invalid("plugin_digest")
	}
	contractDigest, err := DefinitionContractDigest(definition)
	if err != nil {
		return definitionRecord{}, err
	}
	return definitionRecord{
		ConnectorID: definition.ID, PluginID: definition.PluginID, PluginVersion: definition.PluginVersion,
		PluginDigest: pluginDigest, SourceDialect: definition.SourceDialect, SchemaVersion: definition.SourceSchemaVersion,
		ProviderID: definition.ProviderID, DriverID: definition.DriverID, Classification: string(definition.Classification),
		Display: string(display), KeySchema: string(keySchema), CapabilitySummary: string(capabilities),
		ContractDigest: contractDigest, AcceptedAt: now, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func digestJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
