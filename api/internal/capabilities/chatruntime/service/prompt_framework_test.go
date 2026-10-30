package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// The time Skill joins every run that uses tools, but no direct chat and no
// organization that disabled it (ADR 0043).
func TestInherentTimeSkillJoinsRunsThatUseTools(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{{ID: skills.SkillTime}, {ID: skills.SkillCalculator}}
	orgEnabled := []string{skills.SkillTime, skills.SkillCalculator}
	cases := []struct {
		name       string
		enabled    []string
		caller     string
		orgEnabled []string
		want       []string
	}{
		{"agent without skills", nil, runtimemodel.ConversationCallerAgent, orgEnabled, []string{skills.SkillTime}},
		{"digital human", []string{skills.SkillCalculator}, runtimemodel.ConversationCallerDigitalHuman, orgEnabled, []string{skills.SkillCalculator, skills.SkillTime}},
		{"ai chat direct chat", nil, runtimemodel.ConversationCallerAIChat, orgEnabled, nil},
		{"ai chat with a skill", []string{skills.SkillCalculator}, runtimemodel.ConversationCallerAIChat, orgEnabled, []string{skills.SkillCalculator, skills.SkillTime}},
		{"already selected", []string{skills.SkillTime}, runtimemodel.ConversationCallerAgent, orgEnabled, []string{skills.SkillTime}},
		{"disabled by the organization", nil, runtimemodel.ConversationCallerAgent, []string{skills.SkillCalculator}, nil},
	}
	for _, tc := range cases {
		got := withInherentTimeSkill(tc.enabled, catalog, tc.orgEnabled, tc.caller, &chatRequestParts{})
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: skills = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

// Every run is told the time in its turn context; a continuation soon after
// its turn started keeps that time, and a caller with its own time guidance
// gets none (ADR 0043).
func TestCurrentTimeContextGoesInTheTurnContext(t *testing.T) {
	s := &service{}
	now := time.Date(2026, 10, 30, 6, 30, 0, 0, time.UTC)
	parts := &chatRequestParts{UserTimezone: "America/New_York", TurnContext: "Channel notes."}
	s.applyCurrentTimeContext(context.Background(), Scope{}, parts, now)
	message := turnContextMessage(parts)
	content, _ := message.Content.(string)
	if !strings.HasPrefix(content, "Current time: 2026-10-30 02:30, Friday (timezone America/New_York, UTC-04:00).") || !strings.HasSuffix(content, "Channel notes.") {
		t.Fatalf("turn context = %q, want the local time first and the caller's context after it", content)
	}

	started := now.Add(-5 * time.Minute)
	parts = &chatRequestParts{UserTimezone: "UTC", TurnStartedAt: started}
	s.applyCurrentTimeContext(context.Background(), Scope{}, parts, now)
	if !strings.Contains(parts.CurrentTimeContext, "06:25") {
		t.Fatalf("continuation time = %q, want the turn's start time", parts.CurrentTimeContext)
	}
	parts = &chatRequestParts{UserTimezone: "UTC", TurnStartedAt: now.Add(-20 * time.Minute)}
	s.applyCurrentTimeContext(context.Background(), Scope{}, parts, now)
	if !strings.Contains(parts.CurrentTimeContext, "06:30") {
		t.Fatalf("late continuation time = %q, want the current time", parts.CurrentTimeContext)
	}

	parts = &chatRequestParts{}
	s.applyCurrentTimeContext(context.Background(), Scope{}, parts, now)
	if parts.UserTimezone != defaultTurnTimezone || !strings.Contains(parts.CurrentTimeContext, "14:30") {
		t.Fatalf("default timezone = %q, context %q; want %s", parts.UserTimezone, parts.CurrentTimeContext, defaultTurnTimezone)
	}

	parts = &chatRequestParts{CurrentTimeProvided: true, TurnContext: "Current time from the caller."}
	s.applyCurrentTimeContext(context.Background(), Scope{}, parts, now)
	if parts.CurrentTimeContext != "" {
		t.Fatalf("time context = %q, want none when the caller provides it", parts.CurrentTimeContext)
	}
}

// Memory edits are left out of the execution notes; the memory files show
// them (ADR 0043).
func TestExecutionNotesLeaveOutMemoryTools(t *testing.T) {
	got := withoutMemoryToolInvocations([]map[string]interface{}{
		{"skill_id": skills.SkillAgentMemory, "tool_name": "memory_edit"},
		{"skill_id": "file-generator", "tool_name": "generate_file"},
		{"skill_id": skills.SkillDigitalHumanMemory, "tool_name": "memory_edit"},
	})
	if len(got) != 1 || got[0]["skill_id"] != "file-generator" {
		t.Fatalf("invocations = %#v, want only the file generator", got)
	}
}
