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

func TestFinalAnswerPlanSnapshotSchemaIsAdvisory(t *testing.T) {
	tool := finalAnswerMetaTool()
	parameters, _ := tool.Function.Parameters.(map[string]interface{})
	properties, _ := parameters["properties"].(map[string]interface{})
	plan, _ := properties["plan"].(map[string]interface{})
	items, _ := plan["items"].(map[string]interface{})
	itemProperties, _ := items["properties"].(map[string]interface{})
	status, _ := itemProperties["status"].(map[string]interface{})
	enum, _ := status["enum"].([]string)
	want := map[string]bool{"pending": false, "in_progress": false, "completed": false, "skipped": false}
	for _, value := range enum {
		if _, ok := want[value]; ok {
			want[value] = true
		}
	}
	for value, found := range want {
		if !found {
			t.Fatalf("final answer plan status enum = %#v, missing %q", enum, value)
		}
	}
}
