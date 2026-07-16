package workflowtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type appTypeCaptureLLMClient struct {
	llmclient.LLMClient
	appTypes []string
	content  string
}

func (c *appTypeCaptureLLMClient) AppChat(_ context.Context, appCtx *llmclient.AppContext, _ *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	c.appTypes = append(c.appTypes, appCtx.AppType)
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{Message: adapter.Message{Content: c.content}}},
	}, nil
}

func TestWorkflowTestLLMCallsUseWorkflowAppType(t *testing.T) {
	t.Run("case generator", func(t *testing.T) {
		client := &appTypeCaptureLLMClient{content: `{"cases":[{"content":"测试问题","expected_result":"预期结果","question_type":"core"}]}`}
		generator := &LLMCaseGenerator{Client: client}

		_, err := generator.GenerateCases(context.Background(), GenerateCasesRequest{})

		require.NoError(t, err)
		require.Equal(t, []string{"workflow"}, client.appTypes)
	})

	t.Run("scenario recognizer", func(t *testing.T) {
		client := &appTypeCaptureLLMClient{content: `{"scenarios":[{"name":"售前咨询","description":"产品咨询"}]}`}
		recognizer := &LLMScenarioRecognizer{Client: client}

		_, err := recognizer.RecognizeScenarios(context.Background(), ScenarioRecognitionInput{})

		require.NoError(t, err)
		require.Equal(t, []string{"workflow"}, client.appTypes)
	})

	t.Run("judge", func(t *testing.T) {
		client := &appTypeCaptureLLMClient{content: `{"result":"通过","reason":"回答有效","confidence":0.9}`}
		judge := &LLMJudge{Client: client}

		_, err := judge.JudgeCase(context.Background(), JudgeRequest{})

		require.NoError(t, err)
		require.Equal(t, []string{"workflow"}, client.appTypes)
	})

	t.Run("summarizer", func(t *testing.T) {
		client := &appTypeCaptureLLMClient{content: "测试结论：调用成功"}
		summarizer := &LLMSummarizer{Client: client}

		_, err := summarizer.SummarizeBatch(context.Background(), SummaryRequest{})

		require.NoError(t, err)
		require.Equal(t, []string{"workflow"}, client.appTypes)
	})
}
