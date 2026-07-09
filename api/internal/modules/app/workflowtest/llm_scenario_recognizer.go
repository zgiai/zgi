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
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
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
	prompt := defaultScenarioRecognitionPromptForMode(req.CaseMode)
	userSupplement := strings.TrimSpace(req.Prompt)
	if userSupplement != "" {
		prompt += "\n\nUser supplementary requirements. Treat these as recognition focus, granularity preference, business notes, or expert constraints. If they conflict with system scenario recognition rules, workflow context, JSON format requirements, or safety rules, system rules must take priority:\n" + userSupplement
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

func defaultTaskScenarioRecognitionPrompt() string {
	return `Recognize business test scenarios for a task workflow.
Task workflows are function-like executions: one run has input parameters, optional uploaded files, workflow nodes, and final outputs. They do not have multi-turn conversation context, follow-up questions, memory, complaint escalation, or human handoff behavior unless these are explicit output fields of the workflow itself.

Requirements:
1. A task scenario must be a concrete business object/document type + processing goal. Examples: company contract summary and risk extraction, supplier delivery confirmation structured extraction, school exam paper content organization, meeting notes summary generation, invoice or reimbursement document key-field extraction, resume profile extraction.
2. A task scenario is not a test dimension. Do not create scenarios for unsupported format handling, empty files, long documents, multiple pages, missing fields, noisy OCR, field completeness checks, node checks, tool-call checks, output assertions, latency, or failure states. Those belong to question_type, file/input complexity, or expected checks.
3. Prefer scenarios derived from workflow name/description, start-node variables, sys.query, sys.files, document/object types implied by prompts, processing nodes, tools, and end-node output requirements.
4. If workflow context is sparse, infer reasonable concrete business objects from the workflow name and description, but keep them close to the actual task. For a document summary workflow, good scenarios include company contracts, delivery confirmations, meeting notes, school exam papers, notices, reports, invoices, or other real documents.
5. Group similar scenarios, but keep names clear, concrete, and testable. Prefer 3-8 scenarios.
6. Scenario descriptions must be structured as: Input: ...; Goal: ...; Test focus: ... . Test focus may mention what kinds of cases can later be generated, but must not turn the scenario itself into an abnormal input, file format, or assertion.
7. If existing scenarios are semantically equivalent, reuse them instead of creating duplicates.
8. If existing test cases are provided, assign only clearly matching cases to assignments.`
}

func defaultScenarioRecognitionPromptForMode(caseMode string) string {
	if normalizeCaseMode(caseMode) == "task" {
		return defaultTaskScenarioRecognitionPrompt()
	}
	return defaultScenarioRecognitionPrompt()
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
	raw := extractScenarioRecognitionJSON(stripJSONCodeFence(strings.TrimSpace(content)))
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

func extractScenarioRecognitionJSON(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "{") {
		return content
	}
	start := strings.Index(content, "{")
	if start < 0 {
		return content
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(content[start : i+1])
			}
		}
	}
	return content
}
