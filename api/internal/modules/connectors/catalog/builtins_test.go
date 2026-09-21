package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestOwnedBuiltinsPreserveAcceptedExecutableContracts(t *testing.T) {
	data, err := os.ReadFile("testdata/builtin-contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var baseline []Definition
	if json.Unmarshal(data, &baseline) != nil {
		t.Fatal("invalid baseline")
	}
	current, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 10 || len(baseline) != 2 {
		t.Fatal("missing builtin")
	}
	count := 0
	for i, definition := range current[:2] {
		if definition.PluginVersion != map[string]string{"github": "1.1.1", "web-search": "1.2.1"}[definition.ID] {
			t.Fatal("native Skill transition requires a new plugin version")
		}
		encoded, _ := json.Marshal(definition)
		var normalized Definition
		if json.Unmarshal(encoded, &normalized) != nil {
			t.Fatal("clone definition")
		}
		normalized.PluginVersion = baseline[i].PluginVersion
		for j := range normalized.Operations {
			if normalized.Operations[j].SkillID != "connector-runtime" {
				t.Fatal("builtin uses retired Skill")
			}
			normalized.Operations[j].SkillID = baseline[i].Operations[j].SkillID
		}
		actual, actualErr := DefinitionContractDigest(normalized)
		expected, expectedErr := DefinitionContractDigest(baseline[i])
		if actualErr != nil || expectedErr != nil || actual != expected {
			t.Fatalf("contract drift: %s", definition.ID)
		}
		if definition.NameI18n["zh-Hans"] == "" || definition.NameI18n["en-US"] == "" {
			t.Fatal("missing localization")
		}
		count += len(definition.Operations)
		if ValidateBuiltinPackage([]Definition{definition}) != nil {
			t.Fatal("builtin rejected")
		}
		definition.Operations[0].ToolName = "forged_tool"
		if ValidateBuiltinPackage([]Definition{definition}) == nil {
			t.Fatal("forged runtime contract admitted")
		}
	}
	if count != 10 {
		t.Fatalf("operations=%d", count)
	}
}

func TestValidateBuiltinPackageAdmitsDigitalHumanProjection(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var base Definition
	for _, definition := range definitions {
		if definition.ID == "github" {
			base = definition
			break
		}
	}
	if base.ID == "" {
		t.Fatal("github builtin missing")
	}
	candidate, err := WithDigitalHumanSurface(base, base.PluginVersion+"-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateBuiltinPackage([]Definition{candidate}); err != nil {
		t.Fatalf("digital human builtin projection rejected: %v", err)
	}

	// The host release is bound to the exact version derived from the builtin.
	candidate.PluginVersion = base.PluginVersion + "-digitalhuman.2"
	if err := ValidateBuiltinPackage([]Definition{candidate}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unbound digital human version = %v, want invalid definition", err)
	}
}

// TestValidateBuiltinPackageAdmitsCurrentV4DigitalHumanBuiltin pins the catalog
// admission of the already-V4 SSH Server Operations built-in. An already-V4
// Digital-Human-only built-in has no pre-V4 base to project onto, so its package
// is admitted only as the exact current built-in: it must not be upgraded
// through WithDigitalHumanSurface with a derived version, and a forged plugin
// version, operation, credential, or other immutable contract field is rejected.
// Admission here is contract-only; this package registers no provider or driver.
func TestValidateBuiltinPackageAdmitsCurrentV4DigitalHumanBuiltin(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var current Definition
	for _, definition := range definitions {
		if definition.IsServerOpsSSHCredentials() {
			current = definition
			break
		}
	}
	if !alreadyV4DigitalHumanOnly(current) {
		t.Fatalf("SSH built-in is not the current V4 Digital-Human-only contract: %+v", current)
	}

	// The plugin loader always stamps an admitted package with its manifest
	// digest, so the candidate carries one even though the built-in does not.
	admitted := cloneDefinition(current)
	admitted.PluginDigest = strings.Repeat("a", 64)
	if err := ValidateBuiltinPackage([]Definition{admitted}); err != nil {
		t.Fatalf("current V4 Digital-Human-only built-in package rejected: %v", err)
	}

	for name, mutate := range map[string]func(*Definition){
		"derived digital human version": func(d *Definition) { d.PluginVersion += "-digitalhuman.1" },
		"newer plugin version":          func(d *Definition) { d.PluginVersion = "2.0.0" },
		"forged operation tool":         func(d *Definition) { d.Operations[0].ToolName = "forged_tool" },
		"forged operation effect":       func(d *Definition) { d.Operations[0].Effect = OperationEffectCreate },
		"added operation":               func(d *Definition) { d.Operations = append(d.Operations, additiveOperationFixture()) },
		"removed operation":             func(d *Definition) { d.Operations = d.Operations[:2] },
		"forged credential field":       func(d *Definition) { d.CredentialFields[0].Name = "address" },
		"widened credential bound":      func(d *Definition) { d.CredentialFields[3].MaxLength++ },
		"forged setup handler":          func(d *Definition) { d.Setup.Handler = "other-handler" },
		"forged provider identity":      func(d *Definition) { d.ProviderID = "other" },
		"additive surfaces":             func(d *Definition) { d.Surfaces = append(FirstReleaseSurfaces(), SurfaceDigitalHuman) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneDefinition(current)
			candidate.PluginDigest = strings.Repeat("a", 64)
			mutate(&candidate)
			if err := ValidateBuiltinPackage([]Definition{candidate}); err == nil {
				t.Fatalf("forged SSH built-in package admitted: %+v", candidate)
			}
		})
	}
}

// TestBuiltinSSHPluginManifestAdmitsAsExactCurrentContract follows the
// production built-in path: the catalog definition is materialized as an Agent
// Plugins manifest, loaded back through the only admission parser, and then
// checked by the built-in package policy. The package must be admitted as the
// exact current contract; the loader path never upgrades it.
func TestBuiltinSSHPluginManifestAdmitsAsExactCurrentContract(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var current Definition
	for _, definition := range definitions {
		if definition.IsServerOpsSSHCredentials() {
			current = definition
			break
		}
	}
	if current.ID == "" {
		t.Fatal("SSH built-in missing")
	}
	manifest, err := BuildAgentPluginManifest([]Definition{current})
	if err != nil {
		t.Fatalf("build SSH built-in manifest: %v", err)
	}
	loader, err := NewPluginLoader()
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := loader.LoadManifestPackage(context.Background(), manifest)
	if err != nil {
		t.Fatalf("load SSH built-in manifest: %v", err)
	}
	if len(plugin.Failures) != 0 || len(plugin.Definitions) != 1 {
		t.Fatalf("SSH built-in manifest produced failures=%#v definitions=%d", plugin.Failures, len(plugin.Definitions))
	}
	if plugin.Definitions[0].PluginVersion != current.PluginVersion {
		t.Fatalf("SSH built-in version = %q, want %q", plugin.Definitions[0].PluginVersion, current.PluginVersion)
	}
	if err := ValidateBuiltinPackage(plugin.Definitions); err != nil {
		t.Fatalf("loaded SSH built-in package rejected: %v", err)
	}
}
