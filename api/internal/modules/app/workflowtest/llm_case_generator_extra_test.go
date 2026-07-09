package workflowtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGeneratedCasesReportsIncompleteJSON(t *testing.T) {
	_, err := parseGeneratedCases(`{"cases":[{"content":"hello","question_type":"core"}`)

	require.Error(t, err)
	require.Contains(t, err.Error(), "JSON")
	require.Contains(t, err.Error(), "unexpected end")
}

func TestParseGeneratedCasesExtractsJSONFromText(t *testing.T) {
	result, err := parseGeneratedCases(`{"cases":[{"content":"hello","expected_result":"world","question_type":"core"}]} suffix`)

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Equal(t, "hello", result.Cases[0].Content)
}

func TestMaxCaseGenerationTokensIncreasesForGeneratedFiles(t *testing.T) {
	tokens := maxCaseGenerationTokens(GenerateCasesRequest{
		Count:    10,
		CaseMode: "task",
		FileGeneration: &FileGenerationConfig{
			Enabled:      true,
			Formats:      []string{"docx"},
			FilesPerCase: 1,
		},
	})

	require.Equal(t, llmCaseGenerationMaxTokens, tokens)
}

func TestBuildGenerateCasesPromptAllowsConversationFileGeneration(t *testing.T) {
	prompt := buildGenerateCasesPrompt(GenerateCasesRequest{
		Count:        1,
		CaseMode:     "conversation",
		TurnStrategy: "multi",
		FileGeneration: &FileGenerationConfig{
			Enabled:      true,
			Formats:      []string{"docx"},
			FilesPerCase: 1,
		},
	})

	require.Contains(t, prompt, "conversation workflow supports file attachments")
	require.Contains(t, prompt, "at least three role=user turns")
	require.Contains(t, prompt, "attach them to the first user turn")
	require.Contains(t, prompt, "do not generate task workflow node checks")
	require.NotContains(t, prompt, "This task workflow requires generated input files")
}

func TestBuildGenerateCasesPromptTreatsPromptAsUserSupplement(t *testing.T) {
	prompt := buildGenerateCasesPrompt(GenerateCasesRequest{
		Count:    1,
		CaseMode: "task",
		Prompt:   "Only generate reimbursement boundary cases.",
	})

	require.Contains(t, prompt, "User supplementary requirements")
	require.Contains(t, prompt, "system rules must take priority")
	require.Contains(t, prompt, "Only generate reimbursement boundary cases.")
	require.Contains(t, prompt, "Generate task-workflow test cases")
}

func TestBuildGenerateCasesPromptUsesTaskQuestionTypeSemantics(t *testing.T) {
	prompt := buildGenerateCasesPrompt(GenerateCasesRequest{
		Count:    1,
		CaseMode: "task",
	})

	require.Contains(t, prompt, "manual=failed execution or needs-review output state")
	require.Contains(t, prompt, "manual never means human handoff")
	require.Contains(t, prompt, "core, extension, fuzzy")
	require.Contains(t, prompt, "Keep these four dimensions separate")
	require.Contains(t, prompt, "Business scenario = the concrete business object/document type and processing goal")
	require.Contains(t, prompt, "question_type = the coverage angle")
	require.Contains(t, prompt, "file_generation/input complexity = the input shape")
	require.Contains(t, prompt, "expected_checks = the pass/fail criteria")
}

func TestBuildGenerateCasesPromptAvoidsManualHandlingForTaskFiles(t *testing.T) {
	prompt := buildGenerateCasesPrompt(GenerateCasesRequest{
		Count:    1,
		CaseMode: "task",
		FileGeneration: &FileGenerationConfig{
			Enabled: true,
			Formats: []string{"docx"},
		},
	})

	require.Contains(t, prompt, "要求重新上传")
	require.NotContains(t, prompt, "require manual handling")
}

func TestBuildGenerateCasesPromptConstrainsFixtureLanguage(t *testing.T) {
	prompt := buildGenerateCasesPrompt(GenerateCasesRequest{
		Count:    1,
		CaseMode: "task",
		FileGeneration: &FileGenerationConfig{
			Enabled: true,
			Formats: []string{"docx"},
		},
	})

	require.Contains(t, prompt, "must be arrays of user-facing natural-language strings")
	require.Contains(t, prompt, "For Chinese cases, facts and expected_checks must be fully Chinese")
	require.Contains(t, prompt, "Do not output English label prefixes")
	require.Contains(t, prompt, "complainant:")
	require.Contains(t, prompt, "事件时间线包括")
	require.Contains(t, prompt, "输出应标注订单号和产品名称缺失")
}

func TestExpectedChecksFromFixturesDeduplicatesChecks(t *testing.T) {
	checks := expectedChecksFromFixtures([]GeneratedFileFixture{{
		ExpectedChecks: []string{
			"输出应包含投诉方王丽",
			"输出应包含投诉方王丽",
			"输出应包含事件时间线",
		},
	}}, "")

	require.Equal(t, []string{
		"输出应包含投诉方王丽",
		"输出应包含事件时间线",
	}, checks)
}

func TestGeneratedAssetConversationChecksUseConversationKeys(t *testing.T) {
	item := &GeneratedCase{
		Content:        "Summarize the attached contract",
		ExpectedResult: "The answer should include lessor, lessee, and rent amount",
		Turns: []CaseTurn{
			{Role: "user", Content: "Please summarize this contract", Inputs: JSONMap{}},
			{Role: "user", Content: "What is the rent amount?", Inputs: JSONMap{}},
		},
	}
	fixtures := []GeneratedFileFixture{{
		ExpectedChecks: []string{"lessor", "lessee", "rent amount"},
	}}

	enrichGeneratedAssetConversationChecks(item, fixtures)

	firstInputs := item.Turns[0].Inputs
	require.Nil(t, firstInputs[expectedChecksInputKey])
	turnChecks := conversationExpectedChecksFromInput(firstInputs[turnChecksInputKey])
	require.Len(t, turnChecks.Conditions, 1)
	require.Equal(t, "task_completion", turnChecks.Conditions[0].Type)
	require.Equal(t, []string{"lessor", "lessee", "rent amount"}, turnChecks.Conditions[0].Values)
	globalChecks := conversationExpectedChecksFromInput(firstInputs[conversationChecksInputKey])
	require.Len(t, globalChecks.Conditions, 1)
	require.Equal(t, "context_following", globalChecks.Conditions[0].Type)
}

func TestLLMCaseGeneratorInfersTaskExpectedChecksDuringGeneration(t *testing.T) {
	client := &fakeLLMClient{
		responseContents: []string{
			`{"cases":[{"content":"请读取附件合同并生成结构化摘要","expected_result":"摘要应包含甲方、乙方、合同金额和签署日期","question_type":"core"}]}`,
			`{"must_call_tools":["file_parser"],"output_contains":["甲方","乙方","合同金额","签署日期"],"max_latency_ms":30000}`,
		},
	}
	generator := &LLMCaseGenerator{
		Client:      client,
		WorkspaceID: "workspace-1",
		AccountID:   "account-1",
		AgentID:     "agent-1",
	}

	result, err := generator.GenerateCases(context.Background(), GenerateCasesRequest{
		Count:           1,
		CaseMode:        "task",
		WorkflowContext: "Workflow structure summary:\n1. 文档解析节点 uses file_parser\n2. 摘要生成节点",
	})

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Len(t, result.Cases[0].Turns, 1)
	require.Len(t, client.requests, 2)

	inputs := result.Cases[0].Turns[0].Inputs
	require.Equal(t, "task", inputs[caseModeInputKey])
	checks := expectedChecksFromInput(inputs[expectedChecksInputKey])
	require.ElementsMatch(t, []string{"file_parser"}, checks.MustCallTools)
	require.ElementsMatch(t, []string{"甲方", "乙方", "合同金额", "签署日期"}, checks.OutputContains)
	require.Equal(t, 30000, checks.MaxLatencyMS)
}

func TestLLMCaseGeneratorEnrichesConversationChecksDuringGeneration(t *testing.T) {
	client := &fakeLLMClient{
		responseContent: `{
			"cases": [{
				"content": "咨询制度后继续追问适用范围",
				"expected_result": "智能体应正确回答制度问题并承接上下文",
				"question_type": "core",
				"turns": [
					{
						"role": "user",
						"content": "公司报销制度怎么规定？",
						"expected_result": "应说明报销制度核心规则",
						"turn_checks": {
							"conditions": [{
								"type": "intent_understanding",
								"operator": "passed",
								"values": {"意图": "咨询报销制度"},
								"severity": "critical"
							}]
						}
					},
					{
						"role": "user",
						"content": "那差旅费也一样吗？",
						"expected_result": "应结合上一轮报销制度说明差旅费适用规则"
					}
				],
				"conversation_checks": {
					"conditions": [{
						"type": "context_following",
						"operator": "passed",
						"values": ["第二轮应承接上一轮报销制度语境"],
						"severity": "normal"
					}]
				}
			}]
		}`,
	}
	generator := &LLMCaseGenerator{
		Client:      client,
		WorkspaceID: "workspace-1",
		AccountID:   "account-1",
		AgentID:     "agent-1",
	}

	result, err := generator.GenerateCases(context.Background(), GenerateCasesRequest{
		Count:    1,
		CaseMode: "conversation",
	})

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Len(t, result.Cases[0].Turns, 2)
	require.Len(t, client.requests, 1)

	firstInputs := result.Cases[0].Turns[0].Inputs
	require.Equal(t, "conversation", firstInputs[caseModeInputKey])
	require.Equal(t, "应说明报销制度核心规则", firstInputs[turnExpectationInputKey])
	firstTurnChecks := conversationExpectedChecksFromInput(firstInputs[turnChecksInputKey])
	require.Len(t, firstTurnChecks.Conditions, 1)
	require.Equal(t, "intent_understanding", firstTurnChecks.Conditions[0].Type)
	require.Equal(t, []string{"意图: 咨询报销制度"}, firstTurnChecks.Conditions[0].Values)

	globalChecks := conversationExpectedChecksFromInput(firstInputs[conversationChecksInputKey])
	require.Len(t, globalChecks.Conditions, 1)
	require.Equal(t, "context_following", globalChecks.Conditions[0].Type)

	secondInputs := result.Cases[0].Turns[1].Inputs
	require.Equal(t, "conversation", secondInputs[caseModeInputKey])
	secondTurnChecks := conversationExpectedChecksFromInput(secondInputs[turnChecksInputKey])
	require.Len(t, secondTurnChecks.Conditions, 1)
	require.Equal(t, "reply_contains", secondTurnChecks.Conditions[0].Type)
	require.Equal(t, "system_default", secondTurnChecks.Conditions[0].Source)
	require.Equal(t, []string{"应结合上一轮报销制度说明差旅费适用规则"}, secondTurnChecks.Conditions[0].Values)
}

func TestLLMCaseGeneratorAddsDefaultConversationChecksWhenMissing(t *testing.T) {
	client := &fakeLLMClient{
		responseContent: `{
			"cases": [{
				"content": "先问政策再追问细节",
				"expected_result": "应连续回答并保持上下文一致",
				"question_type": "core",
				"turns": [
					{"role": "user", "content": "年假怎么算？", "expected_result": "应解释年假计算规则"},
					{"role": "user", "content": "入职半年呢？", "expected_result": "应承接年假问题说明入职半年情况"}
				]
			}]
		}`,
	}
	generator := &LLMCaseGenerator{
		Client:      client,
		WorkspaceID: "workspace-1",
		AccountID:   "account-1",
		AgentID:     "agent-1",
	}

	result, err := generator.GenerateCases(context.Background(), GenerateCasesRequest{
		Count:    1,
		CaseMode: "conversation",
	})

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	firstInputs := result.Cases[0].Turns[0].Inputs
	globalChecks := conversationExpectedChecksFromInput(firstInputs[conversationChecksInputKey])
	require.Len(t, globalChecks.Conditions, 1)
	require.Equal(t, "context_following", globalChecks.Conditions[0].Type)
	require.Equal(t, "system_default", globalChecks.Conditions[0].Source)

	firstTurnChecks := conversationExpectedChecksFromInput(firstInputs[turnChecksInputKey])
	require.Equal(t, []string{"应解释年假计算规则"}, firstTurnChecks.Conditions[0].Values)
}

func TestParseGeneratedCasesNormalizesObjectFileFixtureFacts(t *testing.T) {
	result, err := parseGeneratedCases(`{
		"cases": [{
			"content": "请提取附件摘要",
			"expected_result": "应识别文档主题并输出摘要",
			"question_type": "core",
			"file_fixtures": [{
				"format": "docx",
				"title": "接收记录",
				"content": "客户张三提交了合同接收记录。",
				"facts": {"客户": "张三", "文档类型": "合同接收记录"},
				"expected_checks": "输出摘要包含客户和文档类型"
			}]
		}]
	}`)

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Len(t, result.Cases[0].FileFixtures, 1)
	require.ElementsMatch(t, []string{"客户: 张三", "文档类型: 合同接收记录"}, result.Cases[0].FileFixtures[0].Facts)
	require.Equal(t, []string{"输出摘要包含客户和文档类型"}, result.Cases[0].FileFixtures[0].ExpectedChecks)
}
