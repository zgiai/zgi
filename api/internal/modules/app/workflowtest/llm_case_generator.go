package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

type LLMCaseGenerator struct {
	Client      llmclient.LLMClient
	WorkspaceID string
	AccountID   string
	AgentID     string
}

func (g *LLMCaseGenerator) GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error) {
	if g == nil || g.Client == nil {
		return nil, fmt.Errorf("llm case generator is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	temperature := 0.4
	maxTokens := 1200
	chatReq := &adapter.ChatRequest{
		Messages: []adapter.Message{
			{Role: "system", Content: "你是工作流自动化批量测试的问题生成助手。请生成可直接作为用户输入的测试问题。"},
			{Role: "user", Content: buildGenerateCasesPrompt(req)},
		},
		Temperature:    &temperature,
		MaxTokens:      &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}
	if req.Model != nil {
		chatReq.Provider = req.Model.Provider
		chatReq.Model = req.Model.Name
	}
	resp, err := g.Client.AppChat(timeoutCtx, &llmclient.AppContext{
		WorkspaceID:        g.WorkspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace,
		AppID:              g.AgentID,
		AppType:            "agent",
		AccountID:          g.AccountID,
	}, chatReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm case generator returned empty response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("llm case generator returned empty content")
	}
	return parseGeneratedCases(content)
}

func buildGenerateCasesPrompt(req GenerateCasesRequest) string {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = `请基于当前智能体工作流、已识别业务场景和已有测试问题，生成一批可进入问题库的候选测试问题。`
	}
	questionTypes := strings.Join(normalizeQuestionTypes(req.QuestionTypes), ", ")
	if questionTypes == "" {
		questionTypes = "core, extension, fuzzy"
	}
	scenarioID := strings.TrimSpace(req.ScenarioID)
	if scenarioID == "" && len(req.ScenarioIDs) > 0 {
		scenarioID = strings.Join(req.ScenarioIDs, ", ")
	}
	turnStrategy := strings.TrimSpace(req.TurnStrategy)
	if turnStrategy == "" {
		turnStrategy = "mixed"
	}
	context := strings.TrimSpace(req.Context)
	if context == "" {
		context = "无额外业务上下文"
	}
	return fmt.Sprintf(`请生成 %d 条工作流批量测试问题。

要求：
1. 问题必须像真实用户输入，不能写成测试说明。
2. 每条问题只包含一轮用户输入；如果要求多轮对话，请输出最少轮次。
3. 每条问题都必须同时给出预期结果，用于后续自动判题。
4. question_type 只能是 %s 三者之一。
5. 优先生成覆盖不同意图和不同复杂度的问题。
6. 结合场景 ID、对话策略和业务上下文生成问题。
7. 输出 JSON 对象，格式为：{"cases":[{"content":"问题内容","expected_result":"预期结果","question_type":"core"}]}

场景 ID：
%s

对话策略：
%s

生成提示词模板：
%s

业务上下文：
%s`, req.Count, questionTypes, scenarioID, turnStrategy, prompt, context)
}

func normalizeQuestionTypes(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case CaseTypeCore, CaseTypeExtension, CaseTypeFuzzy:
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func parseGeneratedCases(content string) (*GenerateCasesResult, error) {
	raw := stripJSONCodeFence(strings.TrimSpace(content))
	var result GenerateCasesResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	normalized, err := normalizeGeneratedCases(&result)
	if err != nil {
		return nil, err
	}
	return &GenerateCasesResult{Cases: normalized}, nil
}
