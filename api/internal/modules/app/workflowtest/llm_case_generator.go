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

type LLMCaseGenerator struct {
	Client      llmclient.LLMClient
	WorkspaceID string
	AccountID   string
	AgentID     string
}

const llmCaseGenerationTimeout = 3 * time.Minute
const generationPromptCaseContentMaxChars = 160
const generationPromptExpectedResultMaxChars = 240

func (g *LLMCaseGenerator) GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error) {
	if g == nil || g.Client == nil {
		return nil, fmt.Errorf("llm case generator is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, llmCaseGenerationTimeout)
	defer cancel()

	temperature := 0.4
	maxTokens := 2400
	chatReq := &adapter.ChatRequest{
		Messages: []adapter.Message{
			{Role: "system", Content: "你是企业智能体工作流测试问题设计专家。你的任务是生成可直接用于批量测试的真实用户输入，用于验证智能体是否理解用户意图、是否按合理业务逻辑处理、输出是否符合预期。你生成的是测试输入，不是演示问题、功能清单或标准答案。只返回 JSON，不要输出解释、寒暄或 markdown。"},
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
	type promptScenario struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		CaseCount   int    `json:"case_count"`
	}
	type promptCase struct {
		ID             string `json:"id"`
		ScenarioID     string `json:"scenario_id,omitempty"`
		Content        string `json:"content"`
		ExpectedResult string `json:"expected_result,omitempty"`
		QuestionType   string `json:"question_type"`
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = `请基于当前智能体工作流、已识别业务场景和已有测试问题，生成一批可进入问题库的候选测试问题。`
	}
	questionTypes := strings.Join(normalizeQuestionTypes(req.QuestionTypes), ", ")
	if questionTypes == "" {
		questionTypes = "core, extension, fuzzy, manual"
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

	scenarios := make([]promptScenario, 0, len(req.Scenarios))
	for _, scenario := range req.Scenarios {
		scenarios = append(scenarios, promptScenario{
			ID:          strings.TrimSpace(scenario.ID),
			Name:        strings.TrimSpace(scenario.Name),
			Description: strings.TrimSpace(scenario.Description),
			CaseCount:   scenario.CaseCount,
		})
	}
	existingCases := make([]promptCase, 0, len(req.ExistingCases))
	for _, item := range req.ExistingCases {
		scenarioID := ""
		if item.ScenarioID != nil {
			scenarioID = strings.TrimSpace(*item.ScenarioID)
		}
		existingCases = append(existingCases, promptCase{
			ID:             strings.TrimSpace(item.ID),
			ScenarioID:     scenarioID,
			Content:        truncateForPrompt(item.Content, generationPromptCaseContentMaxChars),
			ExpectedResult: truncateForPrompt(item.ExpectedResult, generationPromptExpectedResultMaxChars),
			QuestionType:   strings.TrimSpace(item.QuestionType),
		})
	}
	scenarioJSON, _ := json.Marshal(scenarios)
	existingCaseJSON, _ := json.Marshal(existingCases)
	workflowContext := strings.TrimSpace(req.WorkflowContext)
	if workflowContext == "" {
		workflowContext = "无工作流上下文"
	}

	return fmt.Sprintf(`请生成 %d 条工作流批量测试问题。

要求：
1. 只围绕目标业务场景和目标问题类型生成，不要生成范围外问题。
2. 问题必须像真实用户输入，不能写成测试说明、演示问题或功能清单。
3. 用户输入中不要出现“测试用例、节点、分支、工作流、路径、场景 ID、评分标准”等内部系统术语。
4. 不要预测或输出预期工作流路径；实际路径由测试执行结果产生。
5. 每条问题都必须可执行，并同时给出明确预期结果，用于后续自动判题。
6. 预期结果应描述智能体应该如何处理，例如识别什么意图、是否需要追问、应返回什么、不能编造什么、是否应兜底或转人工。
7. 预期结果不要只写“正确回答”“妥善处理”“完成任务”等空泛描述。
8. question_type 只能是 %s 之一。
9. core 覆盖核心高频主流程；extension 覆盖同一或相近意图的不同表达；fuzzy 覆盖信息缺失、表达模糊、异常或兜底输入；manual 覆盖需要人工介入、人工确认、投诉升级或线下处理的输入。
10. 如果生成数量大于目标业务场景数量，优先保证每个目标业务场景至少 1 条问题；剩余数量在场景和问题类型之间尽量分散。
11. 避免与已有测试问题重复；允许少量有新测试价值的语义变体，但不能只是简单换词。
12. 输出 JSON 对象，格式为：{"cases":[{"scenario_id":"场景ID","content":"问题内容","expected_result":"预期结果","question_type":"core"}]}

目标业务场景 ID：
%s

目标业务场景详情：
%s

对话策略：
%s

工作流上下文（系统自动从当前草稿提取，优先用于理解智能体真实业务）：
%s

已有测试问题（用于避免重复并生成有新增价值的问题）：
%s

用户补充生成要求：
%s

业务上下文：
%s`, req.Count, questionTypes, scenarioID, string(scenarioJSON), turnStrategy, workflowContext, string(existingCaseJSON), prompt, context)
}

func normalizeQuestionTypes(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		questionType := strings.TrimSpace(value)
		switch questionType {
		case CaseTypeCore, CaseTypeExtension, CaseTypeFuzzy, CaseTypeManual:
			if _, ok := seen[questionType]; ok {
				continue
			}
			seen[questionType] = struct{}{}
			result = append(result, questionType)
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
