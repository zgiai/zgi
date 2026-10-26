package runtimeworkspace

import (
	"encoding/json"
	"testing"
)

func TestRuntimeAgentCapabilitiesDoesNotExposeDeferredResources(t *testing.T) {
	capabilities := runtimeAgentCapabilities(runtimePublishedPresentation{
		FileUploadEnabled:   true,
		SupportsVision:      true,
		AgentMemoryEnabled:  true,
		EnabledSkillIDs:     []string{"skill"},
		KnowledgeDatasetIDs: []string{"dataset"},
		DatabaseBindings:    []json.RawMessage{{}},
		WorkflowBindings:    []json.RawMessage{{}},
		ConnectorBindings:   []json.RawMessage{{}},
	})

	allowed := map[string]bool{}
	for _, capability := range capabilities {
		allowed[capability] = true
	}
	for _, capability := range []string{"chat", "conversations", "files", "memory", "vision", "skills"} {
		if !allowed[capability] {
			t.Fatalf("capability %q missing from %#v", capability, capabilities)
		}
	}
	for _, capability := range []string{"knowledge", "database", "workflow", "connectors"} {
		if allowed[capability] {
			t.Fatalf("deferred capability %q leaked into %#v", capability, capabilities)
		}
	}
}
