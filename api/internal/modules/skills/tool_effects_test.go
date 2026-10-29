package skills

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestToolIsReadOnlyOnlyForDeclaredReadsAndSideEffectFreeSystemSkills(t *testing.T) {
	read := &toolgovernance.Manifest{Effect: toolgovernance.EffectRead}
	externalRead := &toolgovernance.Manifest{Effect: toolgovernance.EffectRead, ExternalSideEffect: true}
	create := &toolgovernance.Manifest{Effect: toolgovernance.EffectCreate}
	doc := func(source, sideEffects string, tool SkillToolDefinition) SkillDocument {
		return SkillDocument{Metadata: SkillMetadata{ID: "s", Source: source, SideEffects: sideEffects}, Tools: []SkillToolDefinition{tool}}
	}
	for name, tt := range map[string]struct {
		doc  SkillDocument
		want bool
	}{
		"governance read":                    {doc(SkillSourceSystem, "", SkillToolDefinition{Name: "t", Governance: read}), true},
		"read with an external side effect":  {doc(SkillSourceSystem, "", SkillToolDefinition{Name: "t", Governance: externalRead}), false},
		"governance create":                  {doc(SkillSourceSystem, SkillSideEffectsNone, SkillToolDefinition{Name: "t", Governance: create}), false},
		"system skill without side effects":  {doc(SkillSourceSystem, SkillSideEffectsNone, SkillToolDefinition{Name: "t"}), true},
		"member skill claiming none":         {doc("custom", SkillSideEffectsNone, SkillToolDefinition{Name: "t"}), false},
		"system skill that declares nothing": {doc(SkillSourceSystem, "", SkillToolDefinition{Name: "t"}), false},
		"connector operation":                {doc(SkillSourceSystem, SkillSideEffectsNone, SkillToolDefinition{Name: "t", Governance: read, ConnectorNative: true}), false},
	} {
		if got := ToolIsReadOnly(tt.doc, "t"); got != tt.want {
			t.Fatalf("%s: ToolIsReadOnly() = %v, want %v", name, got, tt.want)
		}
	}
	if !SystemSkillToolIsReadOnly("calculator", "evaluate_expression") || !SystemSkillToolIsReadOnly("time", "current_time") {
		t.Fatal("the calculator and time skills declare no side effects")
	}
	if SystemSkillToolIsReadOnly("file-generator", "generate_file") {
		t.Fatal("generate_file creates a file")
	}
}
