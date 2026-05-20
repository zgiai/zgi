package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type LLMJudge struct {
	Client      llmclient.LLMClient
	WorkspaceID string
	AccountID   string
	Provider    string
	Model       string
}

func (j *LLMJudge) JudgeCase(ctx context.Context, req JudgeRequest) (*JudgeResult, error) {
	if j == nil || j.Client == nil {
		return nil, fmt.Errorf("llm judge is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	temperature := 0.0
	maxTokens := 800
	resp, err := j.Client.AppChat(timeoutCtx, &llmclient.AppContext{
		WorkspaceID:        j.WorkspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace,
		AppID:              req.AgentID,
		AppType:            "agent",
		AccountID:          j.AccountID,
		SessionID:          req.BatchID,
		WorkflowRunID:      req.RunResult.WorkflowRunID,
	}, &adapter.ChatRequest{
		Provider: j.Provider,
		Model:    j.Model,
		Messages: []adapter.Message{
			{Role: "system", Content: strings.TrimSpace(req.PromptTemplate)},
			{Role: "user", Content: buildJudgeUserPrompt(req)},
		},
		Temperature:    &temperature,
		MaxTokens:      &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm judge returned empty response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("llm judge returned empty content")
	}
	return parseJudgeResponse(content)
}

func buildJudgeUserPrompt(req JudgeRequest) string {
	outputBytes, _ := json.Marshal(req.RunResult.Outputs)
	return fmt.Sprintf(`请基于以下测试样本输出评分 JSON。

输出格式必须是 JSON 对象：
{
  "result": "通过 | 不通过 | 需复核",
  "reason": "一句话说明判断依据",
  "suggestion": "如果不通过或需复核，给出改进建议；通过时可为空",
  "confidence": 0.0
}

测试问题：
%s

预期结果：
%s

多轮输入：
%s

智能体执行结果：
%s`, req.CaseSnapshot.Content, req.CaseSnapshot.ExpectedResult, marshalForPrompt(req.CaseSnapshot.Turns), string(outputBytes))
}

func marshalForPrompt(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

type judgeResponsePayload struct {
	Result     string  `json:"result"`
	Status     string  `json:"status"`
	Reason     string  `json:"reason"`
	Suggestion string  `json:"suggestion"`
	Confidence float64 `json:"confidence"`
}

func parseJudgeResponse(content string) (*JudgeResult, error) {
	raw := stripJSONCodeFence(strings.TrimSpace(content))
	var payload judgeResponsePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	statusText := strings.TrimSpace(payload.Result)
	if statusText == "" {
		statusText = strings.TrimSpace(payload.Status)
	}
	result := &JudgeResult{
		Status:     mapJudgeStatus(statusText),
		Reason:     strings.TrimSpace(payload.Reason),
		Suggestion: strings.TrimSpace(payload.Suggestion),
		Confidence: payload.Confidence,
	}
	return normalizeJudgeResult(result), nil
}

func stripJSONCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```JSON")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func mapJudgeStatus(status string) BatchItemStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "passed", "pass", "通过":
		return BatchItemStatusPassed
	case "failed", "fail", "不通过":
		return BatchItemStatusFailed
	case "review", "needs_review", "需复核", "待复核":
		return BatchItemStatusReview
	default:
		return BatchItemStatusReview
	}
}
