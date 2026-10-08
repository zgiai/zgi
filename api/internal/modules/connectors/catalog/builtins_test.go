package catalog

import (
	"encoding/json"
	"os"
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
