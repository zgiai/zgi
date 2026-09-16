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

func TestWithDigitalHumanSurfaceAdmitsSeparateContract(t *testing.T) {
	base := validDefinition()
	candidate, err := WithDigitalHumanSurface(base, "1.0.0-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	if !candidate.IsDigitalHumanContract() || !candidate.Supports(SurfaceDigitalHuman) {
		t.Fatalf("digital human contract not admitted: %+v", candidate)
	}
	if candidate.PluginVersion != "1.0.0-digitalhuman.1" || candidate.PluginDigest != "" {
		t.Fatalf("plugin identity = %q/%q", candidate.PluginVersion, candidate.PluginDigest)
	}
	if base.IsDigitalHumanContract() || base.PluginVersion != "1.0.0" {
		t.Fatalf("source definition mutated: %+v", base)
	}
	if err := Validate(candidate); err != nil {
		t.Fatalf("admitted contract no longer validates: %v", err)
	}
	for _, pluginVersion := range []string{"", candidate.PluginVersion} {
		if _, err := WithDigitalHumanSurface(candidate, pluginVersion); !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("WithDigitalHumanSurface(%q) = %v, want invalid plugin version", pluginVersion, err)
		}
	}
}

func TestAdditiveDigitalHumanSurfaceOnlyExtendsFirstReleaseContracts(t *testing.T) {
	current := validDefinition()
	candidate, err := WithDigitalHumanSurface(current, "1.0.0-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	if !additiveDigitalHumanSurface(current, candidate) {
		t.Fatal("first-release contract rejected a digital human extension")
	}

	next, err := WithDigitalHumanSurface(current, "1.0.0-digitalhuman.2")
	if err != nil {
		t.Fatal(err)
	}
	if additiveDigitalHumanSurface(candidate, next) {
		t.Fatal("digital human contract admitted a second extension")
	}
	if additiveDigitalHumanSurface(current, current) {
		t.Fatal("identical definitions admitted as an extension")
	}
	if additiveDigitalHumanSurface(current, validDefinition()) {
		t.Fatal("non-digital-human candidate admitted as an extension")
	}
	appCredentials := current
	appCredentials.ExtensionSchemaVersion, appCredentials.ContractVersion = ZGIMetadataSchemaVersion11, 2
	if additiveDigitalHumanSurface(appCredentials, candidate) {
		t.Fatal("credential dialect mismatch admitted as an extension")
	}
}
