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

type LLMScenarioRecognizer struct {
	Client      llmclient.LLMClient
	WorkspaceID string
	AccountID   string
	AgentID     string
}

const scenarioRecognitionCaseContentMaxChars = 160

const llmScenarioRecognitionTimeout = 3 * time.Minute

func (r *LLMScenarioRecognizer) RecognizeScenarios(ctx context.Context, req ScenarioRecognitionInput) (*ScenarioRecognitionResult, error) {
	if r == nil || r.Client == nil {
		return nil, fmt.Errorf("llm scenario recognizer is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, llmScenarioRecognitionTimeout)
	defer cancel()

	temperature := 0.2
	maxTokens := 3200
	chatReq := &adapter.ChatRequest{
		Messages: []adapter.Message{
			{Role: "system", Content: "你是工作流自动化批量测试的业务场景识别助手。请根据工作流结构、节点说明、系统提示词、已有场景和已有测试问题，识别用户真实会触发的业务场景。业务场景是用户意图，不是节点名、分支名、工具名或技术路径。"},
			{Role: "user", Content: buildScenarioRecognitionPrompt(req)},
		},
		Temperature:    &temperature,
		MaxTokens:      &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}
	if req.Model != nil {
		chatReq.Provider = req.Model.Provider
		chatReq.Model = req.Model.Name
	}
	resp, err := r.Client.AppChat(timeoutCtx, &llmclient.AppContext{
		WorkspaceID:        r.WorkspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace,
		AppID:              r.AgentID,
		AppType:            "agent",
		AccountID:          r.AccountID,
	}, chatReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm scenario recognizer returned empty response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("llm scenario recognizer returned empty content")
	}
	return parseScenarioRecognitionResponse(content)
}

func buildScenarioRecognitionPrompt(req ScenarioRecognitionInput) string {
	type promptCase struct {
		ID           string `json:"id"`
		Content      string `json:"content"`
		QuestionType string `json:"question_type"`
	}
	type promptScenario struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	cases := make([]promptCase, 0, len(req.Cases))
	for _, item := range req.Cases {
		cases = append(cases, promptCase{
			ID:           item.ID,
			Content:      truncateForScenarioRecognition(item.Content),
			QuestionType: item.QuestionType,
		})
	}
	existing := make([]promptScenario, 0, len(req.ExistingScenarios))
	for _, item := range req.ExistingScenarios {
		existing = append(existing, promptScenario{
			Name:        item.Name,
			Description: item.Description,
		})
	}
	caseJSON, _ := json.Marshal(cases)
	existingJSON, _ := json.Marshal(existing)
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = defaultScenarioRecognitionPrompt()
	}
	return fmt.Sprintf(`%s

输出 JSON 对象，格式为：{"scenarios":[{"name":"售前咨询","description":"产品能力、价格和采购咨询"}],"assignments":[{"case_id":"问题ID","scenario_name":"售前咨询"}]}

工作流上下文（系统自动从当前草稿提取，优先用于识别场景）：
%s

用户补充业务上下文：
%s

已有场景：
%s

测试问题：
%s`, prompt, strings.TrimSpace(req.WorkflowContext), strings.TrimSpace(req.Context), string(existingJSON), string(caseJSON))
}

func defaultScenarioRecognitionPrompt() string {
	return `请基于当前智能体工作流结构、节点说明和系统提示词，识别用户真实会触发的业务场景。

要求：
1. 业务场景不是节点名或分支名，而是用户意图，例如投诉升级、订单查询、售后退款。
2. 不要把节点名、分支名、工具名直接作为场景名。
3. 合并语义重复的场景，保留清晰、可测试的名称。
4. 每个场景输出名称、判断说明和适合生成测试问题的覆盖角度。
5. 优先覆盖高频、关键、异常和兜底场景。
6. 已有场景名称完全相同或语义高度一致则复用，不要重复创建。
7. 如果提供了测试问题，请把能明确归类的问题分配到 assignments；无法明确归类的问题可以不分配，不要强行归类。`
}

func truncateForScenarioRecognition(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= scenarioRecognitionCaseContentMaxChars {
		return value
	}
	return strings.TrimSpace(string(runes[:scenarioRecognitionCaseContentMaxChars])) + "..."
}

func parseScenarioRecognitionResponse(content string) (*ScenarioRecognitionResult, error) {
	raw := stripJSONCodeFence(strings.TrimSpace(content))
	var result ScenarioRecognitionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("failed to parse scenario recognition JSON: %w", err)
	}
	normalized, err := normalizeScenarioRecognitionResult(&result)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}
