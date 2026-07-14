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
	actualOutput := judgeActualOutputText(req)
	if actualOutput == "" {
		actualOutput = "（未识别到可评分的业务输出）"
	}
	mode := workflowTestModeFromSnapshot(req.CaseSnapshot)
	evaluationSchemaText := "（对话工作流暂不使用任务评价 schema）"
	if mode != "conversation" {
		evaluationSchemaText = marshalForPrompt(judgeTaskEvaluationSchema(req))
	}
	analysisText := judgeStructuredAnalysisText(req)
	return fmt.Sprintf(`请基于以下测试样本输出评分 JSON。

输出格式必须是 JSON 对象：
{
  "result": "通过 | 不通过 | 需复核",
  "reason": "一句话说明判断依据",
  "suggestion": "如果不通过或需复核，给出改进建议；通过时可为空",
  "confidence": 0.0
}

测试类型：
%s

评分边界：
%s

结构化评价标准：
%s

结构化检查摘要：
%s

测试问题：
%s

预期结果：
%s

多轮输入：
%s

智能体执行结果：
%s`, judgeModeLabel(mode), judgeModeScoringRules(mode), evaluationSchemaText, analysisText, req.CaseSnapshot.Content, req.CaseSnapshot.ExpectedResult, marshalForPrompt(req.CaseSnapshot.Turns), actualOutput)
}

func judgeStructuredAnalysisText(req JudgeRequest) string {
	outputs := req.RunResult.Outputs
	if outputs == nil {
		return "（暂无结构化检查摘要）"
	}
	if checks, ok := outputs[checkResultsOutputKey]; ok {
		return marshalForPrompt(checks)
	}
	if analysis, ok := outputs[workflowTestAnalysisOutputKey]; ok {
		return marshalForPrompt(analysis)
	}
	return "（暂无结构化检查摘要）"
}

func judgeActualOutputText(req JudgeRequest) string {
	outputs := req.RunResult.Outputs
	if outputs == nil {
		outputs = map[string]interface{}{}
	}
	if workflowTestModeFromSnapshot(req.CaseSnapshot) == "conversation" {
		return strings.TrimSpace(conversationTranscriptText(req.CaseSnapshot, outputs))
	}
	return strings.TrimSpace(workflowActualOutputText(outputs))
}

func judgeModeLabel(mode string) string {
	if mode == "conversation" {
		return "对话工作流：多轮用户消息与智能体回复"
	}
	return "任务工作流：单次函数式执行，只有输入参数和输出结果"
}

func judgeModeScoringRules(mode string) string {
	if mode == "conversation" {
		return strings.TrimSpace(`1. 对话工作流可以根据多轮上下文评分，应检查是否理解当前轮用户意图、是否保持上下文一致、是否在信息不足时主动澄清。
2. 如果用户问题缺少完成任务必需的信息，合理的追问、澄清或说明限制应视为符合预期；不能要求一次性产出不存在的事实。
3. 不得因为回复没有逐字命中期望表达而判失败；只要业务意图、关键动作和安全边界满足即可通过或需复核。`)
	}
	return strings.TrimSpace(`1. 任务工作流类似函数调用：只能根据本次输入参数、上传文件和工作流执行结果评分，不应期待它拥有额外上下文、历史记忆或继续追问能力。
2. 如果输入材料缺少日期、相关方、金额、联系人等字段，输出中用“缺失、未提供、无、待补充、请补充、未知、N/A、[]占位符”等方式明确标注缺失，通常应视为符合“不得编造”的预期；不得仅因没有使用期望中的同义固定词（如“无日期”）而判不通过。
3. 严格区分两类“缺失”：A. 技术性缺失/解析失败，例如声称文件无法读取、内容为空、格式不可读、需要重新上传或转人工；B. 业务文档未约定的补充信息，例如合同总金额、验收标准、争议解决条款、联系方式等。只有 A 与完整可读文件冲突时才应扣分；B 作为风险或缺失信息栏目列出，通常是合理的结构化分析，不应视为违反“不得提示内容缺失”。
4. 如果期望写着“不得提示内容缺失”，默认理解为不得声称文件解析失败、内容为空或无法处理；除非期望明确写“不要输出缺失信息栏目/不要列出未约定条款”，否则不要因为输出列出业务补充信息而判不通过。
5. 对任务流的评分重点是输出是否忠实反映输入、是否抽取了可得信息、是否清楚标注不可得信息、是否避免编造具体事实。不要要求任务流主动联系用户、转人工、升级处理或补全输入之外的信息，除非测试期望明确要求这些是输出字段。
6. 如果输出包含输入中不存在的具体事实，应扣分；但如果只是以占位符或缺失清单表达需要补充的信息，不应视为幻觉。
7. expected_result 是理想业务结果，不是逐字验收清单；不要因为格式、措辞、字段顺序、补充分析不完全一致而直接判不通过。
8. 只要核心目标完成、主要事实方向正确、没有关键编造或技术性失败，通常应判通过；少量非核心字段缺漏或表达不确定，应优先判通过或需复核。
9. 只有核心任务没有完成、关键事实整体错误、编造输入不存在的关键事实、明明有可读文件却声称无法读取/内容为空/解析失败、或关键节点/工具明显未执行时，才判不通过。`)
}

func judgeTaskEvaluationSchema(req JudgeRequest) TaskEvaluationSchema {
	checks := TaskExpectedChecks{}
	var rawSchema interface{}
	if len(req.CaseSnapshot.Turns) > 0 {
		inputs := req.CaseSnapshot.Turns[0].Inputs
		checks = expectedChecksFromInput(inputs[expectedChecksInputKey])
		rawSchema = inputs[evaluationSchemaInputKey]
	}
	return taskEvaluationSchemaFromInput(rawSchema, req.CaseSnapshot.ExpectedResult, checks)
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
