package service

import (
	"reflect"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// A turn starts with the skills whose tools succeeded in recent turns
// (ADR 0043).
func TestRecentlyUsedSkillIDsComeFromRecentSuccessfulCalls(t *testing.T) {
	turn := func(invocations ...map[string]interface{}) *runtimemodel.Message {
		values := make([]interface{}, 0, len(invocations))
		for _, invocation := range invocations {
			values = append(values, invocation)
		}
		return &runtimemodel.Message{Status: runtimemodel.MessageStatusCompleted, Metadata: map[string]interface{}{"skill_invocations": values}}
	}
	branch := []*runtimemodel.Message{
		turn(map[string]interface{}{"kind": "tool_call", "status": "success", "skill_id": "chart-generator"}),
		turn(),
		turn(map[string]interface{}{"kind": "tool_call", "status": "error", "skill_id": "email-writing"}),
		turn(map[string]interface{}{"kind": "tool_call", "status": "success", "skill_id": "file-generator"}, map[string]interface{}{"kind": "skill_load", "status": "success", "skill_id": "writer"}),
	}
	if got := recentlyUsedSkillIDs(branch, 3); !reflect.DeepEqual(got, []string{"file-generator"}) {
		t.Fatalf("recent skills = %#v, want only the successful call within three turns", got)
	}
	if got := recentlyUsedSkillIDs(branch, 4); !reflect.DeepEqual(got, []string{"file-generator", "chart-generator"}) {
		t.Fatalf("recent skills over four turns = %#v", got)
	}
}
