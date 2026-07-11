package skills

import "testing"

func TestSkillPromptMetadataIncludesExposureAndCallers(t *testing.T) {
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{
			ID:               SkillConsoleNavigator,
			Source:           SkillSourceSystem,
			Name:             "Console Navigator",
			SupportedCallers: []string{SkillCallerAIChat},
		},
	}}}

	metadata := resolved.PromptMetadata()
	if len(metadata) != 1 {
		t.Fatalf("PromptMetadata len = %d, want 1", len(metadata))
	}
	got := metadata[0]
	if got.Exposure.Category != SkillExposureSidebarManaged || got.Exposure.UserSelectable {
		t.Fatalf("exposure = %#v, want sidebar managed and not user selectable", got.Exposure)
	}
	if len(got.SupportedCallers) != 1 || got.SupportedCallers[0] != SkillCallerAIChat {
		t.Fatalf("supported callers = %#v, want [aichat]", got.SupportedCallers)
	}
}

func TestSkillBindableToAgentUsesExposureProfile(t *testing.T) {
	if SkillBindableToAgent(SkillDiscoveryMetadata{
		ID:               SkillIntentRouter,
		SupportedCallers: []string{SkillCallerAgent},
	}) {
		t.Fatal("SkillBindableToAgent(intent-router) = true, want false")
	}

	if !SkillBindableToAgent(SkillDiscoveryMetadata{
		ID:               SkillFileGenerator,
		SupportedCallers: []string{SkillCallerAgent},
	}) {
		t.Fatal("SkillBindableToAgent(file-generator) = false, want true")
	}
}
