package workflowtest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type judgeLLMClientStub struct {
	llmclient.LLMClient
	req *adapter.ChatRequest
}

func (s *judgeLLMClientStub) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	s.req = req
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Content: `{"result":"通过","reason":"回答有效","suggestion":"","confidence":0.91}`},
		}},
	}, nil
}

func TestParseJudgeResponseAcceptsJSONCodeFence(t *testing.T) {
	result, err := parseJudgeResponse("```json\n{\"result\":\"不通过\",\"reason\":\"答非所问\",\"suggestion\":\"补充订单状态\",\"confidence\":0.82}\n```")

	require.NoError(t, err)
	require.Equal(t, BatchItemStatusFailed, result.Status)
	require.Equal(t, "答非所问", result.Reason)
	require.Equal(t, "补充订单状态", result.Suggestion)
	require.Equal(t, 0.82, result.Confidence)
}

func TestParseJudgeResponseNormalizesStatusAndConfidence(t *testing.T) {
	result, err := parseJudgeResponse(`{"status":"需复核","reason":"边界模糊","confidence":1.7}`)

	require.NoError(t, err)
	require.Equal(t, BatchItemStatusReview, result.Status)
	require.Equal(t, 1.0, result.Confidence)
}

func TestParseJudgeResponseRejectsInvalidJSON(t *testing.T) {
	_, err := parseJudgeResponse("模型返回了一段无法解析的文字")

	require.Error(t, err)
}

func TestLLMJudgePassesConfiguredModel(t *testing.T) {
	client := &judgeLLMClientStub{}
	judge := &LLMJudge{
		Client:      client,
		WorkspaceID: "workspace-1",
		AccountID:   "account-1",
		Provider:    "openai",
		Model:       "chatgpt-4o-latest",
	}

	result, err := judge.JudgeCase(context.Background(), JudgeRequest{
		AgentID:     "agent-1",
		BatchID:     "batch-1",
		BatchItemID: "item-1",
		CaseSnapshot: CaseSnapshot{
			Content:        "请介绍企业版能力",
			ExpectedResult: "应说明企业版支持团队协作和权限控制。",
		},
		RunResult: RunCaseResult{
			WorkflowRunID: "run-1",
			Outputs:       map[string]interface{}{"answer": "企业版支持团队协作"},
		},
		PromptTemplate: DefaultJudgePromptTemplate,
	})

	require.NoError(t, err)
	require.Equal(t, BatchItemStatusPassed, result.Status)
	require.NotNil(t, client.req)
	require.Equal(t, "openai", client.req.Provider)
	require.Equal(t, "chatgpt-4o-latest", client.req.Model)
	require.Contains(t, client.req.Messages[1].Content, "预期结果")
	require.Contains(t, client.req.Messages[1].Content, "应说明企业版支持团队协作和权限控制。")
}

func TestLLMJudgeUsesBusinessOutputInsteadOfRuntimeJSON(t *testing.T) {
	client := &judgeLLMClientStub{}
	judge := &LLMJudge{
		Client:      client,
		WorkspaceID: "workspace-1",
		AccountID:   "account-1",
		Provider:    "openai",
		Model:       "chatgpt-4o-latest",
	}

	_, err := judge.JudgeCase(context.Background(), JudgeRequest{
		AgentID:     "agent-1",
		BatchID:     "batch-1",
		BatchItemID: "item-1",
		CaseSnapshot: CaseSnapshot{
			Content:        "请处理确认单",
			ExpectedResult: "应识别供应商和关键义务。",
			Turns: CaseTurns{{
				Role:    "user",
				Content: "请处理确认单",
				Inputs:  JSONMap{caseModeInputKey: "task"},
			}},
		},
		RunResult: RunCaseResult{
			WorkflowRunID: "run-1",
			Outputs: map[string]interface{}{
				"elapsed_time":    10186.9,
				"status":          "succeeded",
				"workflow_run_id": "run-1",
				"node_results":    map[string]interface{}{"llm_node": map[string]interface{}{"status": "succeeded"}},
				"outputs": map[string]interface{}{
					"summary": "**结构化接收记录**\n\n- 发件方：宏达供应链公司",
				},
			},
		},
		PromptTemplate: DefaultJudgePromptTemplate,
	})

	require.NoError(t, err)
	require.NotNil(t, client.req)
	prompt := client.req.Messages[1].Content
	require.Contains(t, prompt, "**结构化接收记录**")
	require.Contains(t, prompt, "宏达供应链公司")
	require.NotContains(t, prompt, "workflow_run_id")
	require.NotContains(t, prompt, "node_results")
	require.NotContains(t, prompt, "elapsed_time")
}

func TestLLMJudgePromptExplainsTaskWorkflowMissingFieldBoundary(t *testing.T) {
	req := JudgeRequest{
		CaseSnapshot: CaseSnapshot{
			Content:        "我上传了一份接收说明，但内容只有几行字，没写日期也没提对方公司，系统能处理吗？",
			ExpectedResult: "应识别输入对应文件解析失败或内容缺失场景，明确标注缺失信息为无日期和无相关方，不得编造不存在内容。",
			Turns: CaseTurns{{
				Role:    "user",
				Content: "我上传了一份接收说明，但内容只有几行字，没写日期也没提对方公司，系统能处理吗？",
				Inputs:  JSONMap{caseModeInputKey: "task"},
			}},
		},
		RunResult: RunCaseResult{Outputs: map[string]interface{}{
			"outputs": map[string]interface{}{
				"summary": "- 相关方：[请补充发送方名称]\n- 接收日期：[请补充具体接收日期]\n- 缺失信息：材料清单或名称、双方联系人及联系方式",
			},
		}},
	}

	prompt := buildJudgeUserPrompt(req)

	require.Contains(t, prompt, "任务工作流：单次函数式执行")
	require.Contains(t, prompt, "只有输入参数和输出结果")
	require.Contains(t, prompt, "请补充")
	require.Contains(t, prompt, "不应视为幻觉")
	require.Contains(t, prompt, "不得仅因没有使用期望中的同义固定词")
	require.NotContains(t, prompt, "node_results")
}

func TestLLMJudgePromptSeparatesBusinessMissingInfoFromParseFailure(t *testing.T) {
	req := JudgeRequest{
		CaseSnapshot: CaseSnapshot{
			Content:        "请帮我处理这份合同接收文件。",
			ExpectedResult: "智能体应成功解析上传的文件，识别出相关方、日期、事实和风险，并不得提示内容缺失或转人工。",
			Turns: CaseTurns{{
				Role:    "user",
				Content: "请帮我处理这份合同接收文件。",
				Inputs:  JSONMap{caseModeInputKey: "task"},
			}},
		},
		RunResult: RunCaseResult{Outputs: map[string]interface{}{
			"outputs": map[string]interface{}{
				"summary": "结构化接收记录\n相关方：星辰科技有限公司、远航设备制造厂\n缺失信息：合同总金额；验收标准；争议解决机制",
			},
		}},
	}

	prompt := buildJudgeUserPrompt(req)

	require.Contains(t, prompt, "技术性缺失/解析失败")
	require.Contains(t, prompt, "业务文档未约定的补充信息")
	require.Contains(t, prompt, "合同总金额")
	require.Contains(t, prompt, "不要因为输出列出业务补充信息而判不通过")
}

func TestLLMJudgePromptIncludesTaskEvaluationSchema(t *testing.T) {
	req := JudgeRequest{
		CaseSnapshot: CaseSnapshot{
			Content:        "请处理合同接收文件。",
			ExpectedResult: "抽取相关方和关键义务。",
			Turns: CaseTurns{{
				Role:    "user",
				Content: "请处理合同接收文件。",
				Inputs: JSONMap{
					caseModeInputKey: "task",
					evaluationSchemaInputKey: map[string]interface{}{
						"goal_type":         "extract",
						"primary_objective": "抽取结构化接收记录",
						"assertions": []interface{}{
							map[string]interface{}{
								"id":          "party",
								"type":        "must_include",
								"description": "识别相关方",
								"values":      []interface{}{"星辰科技有限公司"},
								"severity":    "critical",
							},
						},
					},
				},
			}},
		},
		RunResult: RunCaseResult{Outputs: map[string]interface{}{"answer": "相关方：星辰科技有限公司"}},
	}

	prompt := buildJudgeUserPrompt(req)

	require.Contains(t, prompt, "结构化评价标准")
	require.Contains(t, prompt, `"goal_type":"extract"`)
	require.Contains(t, prompt, `"primary_objective":"抽取结构化接收记录"`)
	require.Contains(t, prompt, `"type":"must_include"`)
	require.Contains(t, prompt, "星辰科技有限公司")
}

func TestLLMJudgePromptKeepsConversationClarificationBoundary(t *testing.T) {
	req := JudgeRequest{
		CaseSnapshot: CaseSnapshot{
			Content: "我要办活动",
			Turns: CaseTurns{{
				Role:    "user",
				Content: "我要办活动",
				Inputs:  JSONMap{caseModeInputKey: "conversation"},
			}},
		},
		RunResult: RunCaseResult{Outputs: map[string]interface{}{
			"turn_results": []map[string]interface{}{
				{"outputs": map[string]interface{}{"answer": "请问预算是多少？"}},
			},
		}},
	}

	prompt := buildJudgeUserPrompt(req)

	require.Contains(t, prompt, "对话工作流：多轮用户消息与智能体回复")
	require.Contains(t, prompt, "是否在信息不足时主动澄清")
	require.Contains(t, prompt, "对话工作流暂不使用任务评价 schema")
	require.Contains(t, prompt, "请问预算是多少？")
}

type failingJudge struct {
	err error
}

func (j failingJudge) JudgeCase(ctx context.Context, req JudgeRequest) (*JudgeResult, error) {
	return nil, j.err
}

func TestRunJudgeReturnsUserFacingReasonWhenModelIsUnavailable(t *testing.T) {
	result := runJudge(context.Background(), failingJudge{err: errors.New("model field is required")}, JudgeRequest{})

	require.Equal(t, BatchItemStatusReview, result.Status)
	require.Equal(t, "AI 评分失败：当前评分模型不可用，请前往默认模型管理修改默认文本模型后重新测试。", result.Reason)
	require.Equal(t, "请在默认模型管理中更换可用的默认文本模型，或在 AI 评分设置中指定其他可用模型后重新执行。", result.Suggestion)
}

func TestRunJudgeReturnsGenericUserFacingReasonForUnknownError(t *testing.T) {
	result := runJudge(context.Background(), failingJudge{err: errors.New("temporary network failure")}, JudgeRequest{})

	require.Equal(t, BatchItemStatusReview, result.Status)
	require.Equal(t, "AI 评分失败，请人工复核本次结果。", result.Reason)
	require.Equal(t, "AI 评分失败，请人工复核或重新测试。", result.Suggestion)
}
