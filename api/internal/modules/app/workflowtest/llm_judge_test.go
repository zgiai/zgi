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

type failingJudge struct {
	err error
}

func (j failingJudge) JudgeCase(ctx context.Context, req JudgeRequest) (*JudgeResult, error) {
	return nil, j.err
}

func TestRunJudgeReturnsStableEnglishReasonWhenModelIsMissing(t *testing.T) {
	result := runJudge(context.Background(), failingJudge{err: errors.New("model field is required")}, JudgeRequest{})

	require.Equal(t, BatchItemStatusReview, result.Status)
	require.Equal(t, "judge failed: model field is required", result.Reason)
	require.Equal(t, "AI scoring failed; review manually or rerun the test", result.Suggestion)
}
