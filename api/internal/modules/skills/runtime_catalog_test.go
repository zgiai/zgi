package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeSystemSkillsExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillInternalKnowledge, SkillAgentKnowledge})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	internal, ok := resolved.Get(SkillInternalKnowledge)
	if !ok {
		t.Fatalf("internal knowledge skill was not resolved")
	}
	if got := toolNames(internal.Tools); !sameStrings(got, []string{"list_accessible_knowledge_bases", "retrieve_knowledge"}) {
		t.Fatalf("internal knowledge tools = %v", got)
	}
	agent, ok := resolved.Get(SkillAgentKnowledge)
	if !ok {
		t.Fatalf("agent knowledge skill was not resolved")
	}
	if got := toolNames(agent.Tools); !sameStrings(got, []string{"retrieve_agent_knowledge"}) {
		t.Fatalf("agent knowledge tools = %v", got)
	}
}

func TestCustomSkillCannotDeclareTools(t *testing.T) {
	root := t.TempDir()
	content := `---
name: custom-tool-skill
description: Invalid custom skill.
when_to_use: Never.
provider_type: builtin
provider_id: knowledge
runtime_type: prompt
tools:
  - retrieve_knowledge
---

# Invalid

This custom skill should not be allowed to declare tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadCustomSkillDocument(root); err == nil {
		t.Fatalf("LoadCustomSkillDocument() error = nil, want custom tool declaration rejection")
	}
}

func toolNames(tools []SkillToolDefinition) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
