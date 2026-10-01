package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestStoredDigitalHumanContractsPreserveAuthenticationAndDigest(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range definitions {
		// Mail V5 intentionally cannot be recast as the earlier V4 dialect.
		if base.IsMailCredentials() {
			continue
		}
		t.Run(base.ID, func(t *testing.T) {
			d := cloneDefinition(base)
			d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion13, 4
			d.PluginVersion += "-digitalhuman.1"
			d.PluginDigest = strings.Repeat("b", 64)
			d.Surfaces = append(d.Surfaces, SurfaceDigitalHuman)
			record, err := definitionAdmissionRecord(d, testTime)
			if err != nil {
				t.Fatal(err)
			}
			row := storedDefinitionRow{
				ConnectorID: record.ConnectorID, PluginID: record.PluginID, PluginVersion: record.PluginVersion,
				PluginDigest: record.PluginDigest, SourceDialect: record.SourceDialect, SchemaVersion: record.SchemaVersion,
				ProviderID: record.ProviderID, DriverID: record.DriverID, Classification: record.Classification,
				Display: record.Display, KeySchema: record.KeySchema, CapabilitySummary: record.CapabilitySummary,
				ContractDigest: record.ContractDigest, Status: string(PluginDefinitionStatusActive),
			}
			loaded, err := row.definition()
			if err != nil {
				t.Fatal(err)
			}
			if !loaded.IsDigitalHumanContract() || loaded.IsAppCredentials() != base.IsAppCredentials() ||
				loaded.IsMessagingCredentials() != base.IsMessagingCredentials() {
				t.Fatal("stored extension changed authentication family")
			}
			row.ContractDigest = strings.Repeat("0", 64)
			if _, err := row.definition(); !errors.Is(err, ErrIncompatibleStoredDefinition) {
				t.Fatalf("tampered digest accepted: %v", err)
			}
			d.ContractVersion = 3
			if err := Validate(d); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("mismatched schema/version accepted: %v", err)
			}
		})
	}
}
