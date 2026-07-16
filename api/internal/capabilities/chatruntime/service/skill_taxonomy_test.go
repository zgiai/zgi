package service

import (
	"reflect"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestCustomSkillDisplayTaxonomyRoundTripsThroughStoredMap(t *testing.T) {
	display := skills.SkillDisplayMetadata{
		Category:  "document_processing",
		Scenarios: []string{"document_handling", "legal_compliance"},
	}
	item := &runtimemodel.CustomSkill{Display: skillDisplayMap(display)}

	got := customSkillDisplayFromRecord(item)
	if got.Category != display.Category {
		t.Fatalf("category = %q, want %q", got.Category, display.Category)
	}
	if !reflect.DeepEqual(got.Scenarios, display.Scenarios) {
		t.Fatalf("scenarios = %#v, want %#v", got.Scenarios, display.Scenarios)
	}
}
