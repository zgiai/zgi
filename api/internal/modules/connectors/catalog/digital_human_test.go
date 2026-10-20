package catalog

import (
	"errors"
	"reflect"
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
		// An already-V4 built-in is its own Digital-Human contract. It has no
		// pre-V4 base to project onto, so re-projecting it would duplicate its
		// surface; its stored round trip is covered by the SSH contract test.
		if base.IsDigitalHumanContract() {
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

func TestDigitalHumanOnlyV4ContractAdmitsAndScopesSurfaces(t *testing.T) {
	t.Parallel()

	digitalHumanOnly := validDefinition()
	digitalHumanOnly.ExtensionSchemaVersion, digitalHumanOnly.ContractVersion = ZGIMetadataSchemaVersion13, 4
	digitalHumanOnly.Surfaces = []Surface{SurfaceDigitalHuman}

	if err := Validate(digitalHumanOnly); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	admitted, err := New(digitalHumanOnly)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	stored, ok := admitted.Get(digitalHumanOnly.ID)
	if !ok {
		t.Fatalf("Get(%q) did not find admitted definition", digitalHumanOnly.ID)
	}
	if !stored.IsDigitalHumanContract() {
		t.Fatalf("admitted definition is not a V4 digital human contract: %+v", stored)
	}
	if !stored.Supports(SurfaceDigitalHuman) {
		t.Fatal("Supports(digital_human) = false, want true")
	}
	for _, surface := range []Surface{SurfaceChat, SurfaceAgent, SurfaceWorkflow} {
		if stored.Supports(surface) {
			t.Fatalf("Supports(%q) = true, want false", surface)
		}
	}

	additive := validDefinition()
	additive.ExtensionSchemaVersion, additive.ContractVersion = ZGIMetadataSchemaVersion13, 4
	additive.Surfaces = append(FirstReleaseSurfaces(), SurfaceDigitalHuman)
	if err := Validate(additive); err != nil {
		t.Fatalf("Validate(additive) error = %v", err)
	}
	if _, err := New(additive); err != nil {
		t.Fatalf("New(additive) error = %v", err)
	}
	for _, surface := range append(FirstReleaseSurfaces(), SurfaceDigitalHuman) {
		if !additive.Supports(surface) {
			t.Fatalf("additive Supports(%q) = false, want true", surface)
		}
	}
}

func TestDigitalHumanSurfaceValidationRejectsMalformedContracts(t *testing.T) {
	t.Parallel()

	digitalHumanContract := func(surfaces ...Surface) Definition {
		definition := validDefinition()
		definition.ExtensionSchemaVersion, definition.ContractVersion = ZGIMetadataSchemaVersion13, 4
		definition.Surfaces = surfaces
		return definition
	}
	nonV4DigitalHumanOnly := validDefinition()
	nonV4DigitalHumanOnly.Surfaces = []Surface{SurfaceDigitalHuman}

	tests := []struct {
		name       string
		definition Definition
	}{
		{
			name:       "digital human plus incomplete base",
			definition: digitalHumanContract(SurfaceDigitalHuman, SurfaceChat, SurfaceAgent),
		},
		{
			name:       "duplicate digital human only",
			definition: digitalHumanContract(SurfaceDigitalHuman, SurfaceDigitalHuman),
		},
		{
			name:       "duplicate digital human in additive set",
			definition: digitalHumanContract(SurfaceChat, SurfaceAgent, SurfaceWorkflow, SurfaceDigitalHuman, SurfaceDigitalHuman),
		},
		{
			name:       "unknown surface beside digital human",
			definition: digitalHumanContract(SurfaceDigitalHuman, "unknown"),
		},
		{
			name:       "unknown surface in additive set",
			definition: digitalHumanContract(SurfaceChat, SurfaceAgent, SurfaceWorkflow, "unknown"),
		},
		{
			name:       "duplicate base surface in additive set",
			definition: digitalHumanContract(SurfaceDigitalHuman, SurfaceChat, SurfaceAgent, SurfaceChat),
		},
		{
			name:       "non-v4 digital human only",
			definition: nonV4DigitalHumanOnly,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := Validate(testCase.definition); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidDefinition)
			}
			if _, err := New(testCase.definition); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("New() error = %v, want %v", err, ErrInvalidDefinition)
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
	digitalHumanOnly := current
	digitalHumanOnly.ExtensionSchemaVersion, digitalHumanOnly.ContractVersion = ZGIMetadataSchemaVersion13, 4
	digitalHumanOnly.PluginVersion = "1.0.0-digitalhuman.1"
	digitalHumanOnly.Surfaces = []Surface{SurfaceDigitalHuman}
	if additiveDigitalHumanSurface(current, digitalHumanOnly) {
		t.Fatal("digital-human-only candidate admitted as an additive extension")
	}
}

func TestMailDigitalHumanProjectionPreservesV5AndV6Contracts(t *testing.T) {
	for _, id := range []string{"gmail", "qqmail", "netease163"} {
		t.Run(id, func(t *testing.T) {
			base, ok := MailDefinition(id)
			if !ok {
				t.Fatal("mail definition missing")
			}
			projected, err := WithDigitalHumanSurface(base, base.PluginVersion+"-digitalhuman.1")
			if err != nil {
				t.Fatal(err)
			}
			if !projected.Supports(SurfaceDigitalHuman) || !projected.IsMailCredentials() ||
				projected.ExtensionSchemaVersion != base.ExtensionSchemaVersion || projected.ContractVersion != base.ContractVersion ||
				projected.SourceDialect != base.SourceDialect || projected.SourceSchemaVersion != base.SourceSchemaVersion ||
				projected.ProviderID != base.ProviderID || projected.DriverID != base.DriverID || projected.AuthMethodID != base.AuthMethodID ||
				projected.SetupMethod != base.SetupMethod || !reflect.DeepEqual(projected.Setup, base.Setup) ||
				!reflect.DeepEqual(projected.CredentialFields, base.CredentialFields) || !reflect.DeepEqual(projected.Operations, base.Operations) {
				t.Fatalf("mail projection changed its V%d contract: %#v", base.ContractVersion, projected)
			}
			if projected.PluginVersion != base.PluginVersion+"-digitalhuman.1" || projected.PluginDigest != "" ||
				!hasAdditiveDigitalHumanSurfaces(projected.Surfaces) || !additiveDigitalHumanSurface(base, projected) {
				t.Fatalf("mail projection identity/surfaces = %q/%q/%v", projected.PluginVersion, projected.PluginDigest, projected.Surfaces)
			}
			if _, err = WithDigitalHumanSurface(projected, projected.PluginVersion+"-digitalhuman.1"); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("mail projection admitted a second projection: %v", err)
			}
		})
	}
}
