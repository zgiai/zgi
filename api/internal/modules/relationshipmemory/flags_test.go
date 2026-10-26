package relationshipmemory

import "testing"

func TestAutomaticExtractionIsFailClosed(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	if AutomaticExtractionEnabled() {
		t.Fatal("automatic extraction must be disabled when unset")
	}

	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "true")
	if !AutomaticExtractionEnabled() {
		t.Fatal("explicit true feature flags were not enabled")
	}
}

func TestAutomaticExtractionAcceptsLegacyFlagForRollingUpgrade(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "true")
	if !AutomaticExtractionEnabled() {
		t.Fatal("legacy automatic extraction flag was not honored")
	}
}
