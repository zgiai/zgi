//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestVisibleSkillMetadataUsesSharedExposureProfile(t *testing.T) {
	metadata := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive},
		{ID: skills.SkillInternalDatabase, Status: skills.SkillStatusActive},
		{ID: skills.SkillInternalKnowledge, Status: skills.SkillStatusActive},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive},
	}

	gotMetadata := visibleSkillMetadata(metadata)
	got := make([]string, 0, len(gotMetadata))
	for _, item := range gotMetadata {
		got = append(got, item.ID)
	}
	want := []string{skills.SkillCalculator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visibleSkillMetadata() = %#v, want %#v", got, want)
	}
}
