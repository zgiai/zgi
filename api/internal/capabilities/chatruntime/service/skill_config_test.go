package service

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestEffectiveAgentSkillIDsAutoAddsHiddenKnowledge(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillInternalKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
		{ID: skills.SkillUserMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentKnowledge, skills.SkillUserMemory, skills.SkillInternalKnowledge},
		catalog,
		&RunConfig{KnowledgeDatasetIDs: []string{"dataset-1"}},
	)
	want := []string{skills.SkillAgentKnowledge, skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsSkipsKnowledgeWithoutDatasets(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentKnowledge},
		catalog,
		&RunConfig{},
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestVisibleSkillMetadataHidesRuntimeManagedSkills(t *testing.T) {
	metadata := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillInternalKnowledge},
		{ID: skills.SkillAgentKnowledge},
		{ID: skills.SkillUserMemory},
		{ID: skills.SkillCalculator},
	}

	got := visibleSkillMetadata(metadata)
	gotIDs := make([]string, 0, len(got))
	for _, item := range got {
		gotIDs = append(gotIDs, item.ID)
	}
	want := []string{skills.SkillInternalKnowledge, skills.SkillCalculator}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("visibleSkillMetadata ids = %#v, want %#v", gotIDs, want)
	}
}
