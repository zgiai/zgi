package relationshipmemory

import "testing"

func TestAutomaticExtractionIsOnUnlessTurnedOff(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	if !AutomaticExtractionEnabled() {
		t.Fatal("automatic memory must be on when unset")
	}
	for _, off := range []string{"false", "0", "off", "No"} {
		t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", off)
		if AutomaticExtractionEnabled() {
			t.Fatalf("%q did not turn automatic memory off", off)
		}
	}
}

func TestAutomaticExtractionHonorsLegacyFlagWhenCurrentIsUnset(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "false")
	if AutomaticExtractionEnabled() {
		t.Fatal("a deployment that turned automatic memory off under the legacy name must stay off")
	}
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "true")
	if !AutomaticExtractionEnabled() {
		t.Fatal("the current variable must win over the legacy one")
	}
}
