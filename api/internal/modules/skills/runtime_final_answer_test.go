package skills

import "testing"

func TestMetaToolsIncludeExplicitFinalAnswer(t *testing.T) {
	for _, tool := range MetaToolsForSkillState(nil, nil) {
		if tool.Function.Name == MetaToolFinalAnswer {
			return
		}
	}
	t.Fatalf("MetaToolsForSkillState() does not include %s", MetaToolFinalAnswer)
}
