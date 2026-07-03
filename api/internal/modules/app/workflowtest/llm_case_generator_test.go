package workflowtest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGeneratedCasesAcceptsJSONCodeFence(t *testing.T) {
	result, err := parseGeneratedCases("```json\n{\"cases\":[{\"content\":\"企业版支持哪些能力？\",\"question_type\":\"核心问题\"},{\"content\":\"如何接入 CRM？\",\"question_type\":\"扩展问法\"},{\"content\":\"我要投诉并找人工处理\",\"question_type\":\"人工介入\"}]}\n```")

	require.NoError(t, err)
	require.Len(t, result.Cases, 3)
	require.Equal(t, CaseTypeCore, result.Cases[0].QuestionType)
	require.Equal(t, CaseTypeExtension, result.Cases[1].QuestionType)
	require.Equal(t, CaseTypeManual, result.Cases[2].QuestionType)
}

func TestParseGeneratedCasesKeepsScenarioID(t *testing.T) {
	result, err := parseGeneratedCases(`{"cases":[{"scenario_id":"scenario-2","content":"订单现在到哪了？","expected_result":"查询订单状态","question_type":"core"}]}`)

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Equal(t, "scenario-2", result.Cases[0].ScenarioID)
	require.Equal(t, "订单现在到哪了？", result.Cases[0].Content)
}

func TestBuildGenerateCasesPromptIncludesWorkflowScenariosAndExistingCases(t *testing.T) {
	scenarioID := "scenario-1"
	prompt := buildGenerateCasesPrompt(GenerateCasesRequest{
		Count:           2,
		ScenarioIDs:     []string{"scenario-1", "scenario-2"},
		QuestionTypes:   []string{CaseTypeCore, CaseTypeFuzzy, CaseTypeManual},
		TurnStrategy:    "mixed",
		Prompt:          "重点覆盖缺失信息和兜底",
		Context:         "面向电商客服",
		WorkflowContext: "Workflow structure summary:\n1. [llm] 客服回复",
		Scenarios: []Scenario{
			{ID: "scenario-1", Name: "订单查询", Description: "查询订单状态", CaseCount: 1},
			{ID: "scenario-2", Name: "售后退款", Description: "处理退款诉求", CaseCount: 0},
		},
		ExistingCases: []Case{
			{ID: "case-1", ScenarioID: &scenarioID, Content: "我的订单到哪里了？", ExpectedResult: "应查询订单状态", QuestionType: CaseTypeCore},
		},
	})

	require.Contains(t, prompt, "Workflow structure summary")
	require.Contains(t, prompt, "订单查询")
	require.Contains(t, prompt, "售后退款")
	require.Contains(t, prompt, "我的订单到哪里了？")
	require.Contains(t, prompt, "scenario_id")
	require.Contains(t, prompt, "不要预测或输出预期工作流路径")
	require.Contains(t, prompt, "重点覆盖缺失信息和兜底")
	require.Contains(t, prompt, "manual")
	require.Contains(t, prompt, "人工介入")
}

func TestSelectExistingCasesForGenerationPromptFiltersAndLimits(t *testing.T) {
	makeCase := func(id, scenarioID string) Case {
		scenarioIDCopy := scenarioID
		return Case{
			ID:             id,
			ScenarioID:     &scenarioIDCopy,
			Content:        "content-" + id,
			ExpectedResult: "expected-" + id,
			QuestionType:   CaseTypeCore,
		}
	}

	cases := make([]Case, 0, 40)
	for i := 0; i < 8; i++ {
		cases = append(cases, makeCase(
			fmt.Sprintf("a-%d", i),
			"scenario-a",
		))
	}
	for i := 0; i < 7; i++ {
		cases = append(cases, makeCase(
			fmt.Sprintf("b-%d", i),
			"scenario-b",
		))
	}
	for i := 0; i < 25; i++ {
		cases = append(cases, makeCase(
			fmt.Sprintf("c-%d", i),
			"scenario-c",
		))
	}

	selected := selectExistingCasesForGenerationPrompt(cases, []string{"scenario-b", "scenario-c"})

	require.Len(t, selected, 10)
	for _, item := range selected {
		require.NotNil(t, item.ScenarioID)
		require.Contains(t, []string{"scenario-b", "scenario-c"}, *item.ScenarioID)
	}
	for i := 0; i < 5; i++ {
		require.Equal(t, "scenario-b", *selected[i].ScenarioID)
	}
	for i := 5; i < 10; i++ {
		require.Equal(t, "scenario-c", *selected[i].ScenarioID)
	}
}

func TestParseGeneratedCasesRejectsInvalidJSON(t *testing.T) {
	_, err := parseGeneratedCases("这里是非 JSON 文本")

	require.Error(t, err)
}
