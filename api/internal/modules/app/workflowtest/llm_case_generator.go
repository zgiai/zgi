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
const llmCaseGenerationMinTokens = 4000
const llmCaseGenerationMaxTokens = 8000
const generationPromptCaseContentMaxChars = 160
const generationPromptExpectedResultMaxChars = 240

func (g *LLMCaseGenerator) GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error) {
	if g == nil || g.Client == nil {
		return nil, fmt.Errorf("llm case generator is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, llmCaseGenerationTimeout)
	defer cancel()

	temperature := 0.4
	maxTokens := maxCaseGenerationTokens(req)
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
	result, err := parseGeneratedCases(content)
	if err != nil {
		return nil, err
	}
	if normalizeCaseMode(req.CaseMode) == "conversation" {
		result = sanitizeConversationGeneratedCases(result)
		if result == nil || len(result.Cases) == 0 {
			return nil, fmt.Errorf("conversation case generator returned assistant replies or scripted dialogue instead of user inputs")
		}
	}
	g.enrichConversationGeneratedChecks(req, result)
	g.enrichTaskGeneratedChecks(timeoutCtx, req, result)
	return result, nil
}

func sanitizeConversationGeneratedCases(result *GenerateCasesResult) *GenerateCasesResult {
	if result == nil {
		return nil
	}
	sanitized := &GenerateCasesResult{Items: result.Items}
	for _, item := range result.Cases {
		turns := sanitizeConversationGeneratedTurns(item.Turns)
		if len(turns) == 0 {
			content := strings.TrimSpace(item.Content)
			if content != "" && !looksLikeAssistantGeneratedTurn(content) {
				turns = []CaseTurn{{Role: "user", Content: content}}
			}
		}
		if len(turns) == 0 {
			continue
		}
		item.Turns = turns
		item.Content = strings.TrimSpace(turns[0].Content)
		sanitized.Cases = append(sanitized.Cases, item)
	}
	return sanitized
}

func sanitizeConversationGeneratedTurns(turns []CaseTurn) []CaseTurn {
	sanitized := make([]CaseTurn, 0, len(turns))
	for _, turn := range turns {
		role := strings.ToLower(strings.TrimSpace(turn.Role))
		if role != "" && role != "user" {
			continue
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" && len(turn.Attachments) == 0 {
			continue
		}
		if looksLikeAssistantGeneratedTurn(content) {
			continue
		}
		turn.Role = "user"
		turn.Content = content
		sanitized = append(sanitized, turn)
	}
	return sanitized
}

func looksLikeAssistantGeneratedTurn(content string) bool {
	text := strings.TrimSpace(content)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	strongPrefixes := []string{
		"已收到", "我已收到", "我将为", "我会为", "我来为", "正在为", "翻译完成", "处理完成", "以下是", "根据你提供", "根据您提供",
		"i have received", "i will now", "here is the result", "here are the results",
	}
	for _, prefix := range strongPrefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}
	strongFragments := []string{
		"请稍候", "收到文件", "收到您上传", "收到你上传", "正在处理", "正在为你", "正在为您", "已为你", "已为您", "为您整理", "为你整理", "翻译如下", "结果如下", "处理结果",
		"please wait", "processing your", "translated as follows", "translation is complete", "the result is as follows",
	}
	for _, fragment := range strongFragments {
		if strings.Contains(lower, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func (g *LLMCaseGenerator) enrichConversationGeneratedChecks(req GenerateCasesRequest, result *GenerateCasesResult) {
	if normalizeCaseMode(req.CaseMode) != "conversation" || result == nil || len(result.Cases) == 0 {
		return
	}
	for index := range result.Cases {
		item := &result.Cases[index]
		if len(item.Turns) == 0 {
			item.Turns = []CaseTurn{{
				Role:    "user",
				Content: strings.TrimSpace(item.Content),
			}}
		}
		for turnIndex := range item.Turns {
			turn := &item.Turns[turnIndex]
			if turn.Inputs == nil {
				turn.Inputs = JSONMap{}
			}
			turn.Inputs[caseModeInputKey] = "conversation"
			checks := conversationExpectedChecksFromInput(turn.Inputs[turnChecksInputKey])
			if !checks.Useful() {
				fallback := firstNonEmptyString(
					stringFromInputValue(turn.Inputs[turnExpectationInputKey]),
					item.ExpectedResult,
					turn.Content,
					item.Content,
				)
				checks = defaultTurnConversationChecks(fallback)
			}
			if checks.Useful() {
				turn.Inputs[turnChecksInputKey] = checks.JSONMap()
			}
		}
		conversationChecks := normalizeConversationExpectedChecks(item.ConversationChecks)
		if !conversationChecks.Useful() && len(item.Turns) > 0 {
			conversationChecks = conversationExpectedChecksFromInput(item.Turns[0].Inputs[conversationChecksInputKey])
		}
		if !conversationChecks.Useful() && len(item.Turns) > 1 {
			conversationChecks = defaultGlobalConversationChecks()
		}
		if conversationChecks.Useful() && len(item.Turns) > 0 {
			item.Turns[0].Inputs[conversationChecksInputKey] = conversationChecks.JSONMap()
		}
		item.ConversationChecks = conversationChecks
	}
}

func (g *LLMCaseGenerator) enrichTaskGeneratedChecks(ctx context.Context, req GenerateCasesRequest, result *GenerateCasesResult) {
	if normalizeCaseMode(req.CaseMode) != "task" || result == nil || len(result.Cases) == 0 {
		return
	}
	inferer := &LLMTaskExpectedCheckInferer{
		Client:      g.Client,
		WorkspaceID: g.WorkspaceID,
		AccountID:   g.AccountID,
		AgentID:     g.AgentID,
	}
	workflowContext := strings.TrimSpace(req.WorkflowContext)
	for index := range result.Cases {
		item := &result.Cases[index]
		if len(item.Turns) == 0 {
			item.Turns = []CaseTurn{{
				Role:    "user",
				Content: strings.TrimSpace(item.Content),
			}}
		}
		turn := &item.Turns[0]
		if turn.Inputs == nil {
			turn.Inputs = JSONMap{}
		}
		turn.Inputs[caseModeInputKey] = "task"
		existingChecks := expectedChecksFromInput(turn.Inputs[expectedChecksInputKey])
		variables := JSONMap{}
		for key, value := range turn.Inputs {
			if strings.HasPrefix(key, "__") {
				continue
			}
			variables[key] = value
		}
		checks := existingChecks
		if !checks.Useful() {
			inferred, err := inferer.Infer(ctx, TaskExpectedCheckInferInput{
				WorkflowContext: workflowContext,
				Content:         firstNonEmptyString(turn.Content, item.Content),
				ExpectedResult:  item.ExpectedResult,
				Variables:       variables,
				Attachments:     turn.Attachments,
			})
			if err == nil && inferred != nil && inferred.Useful() {
				checks = *inferred
				turn.Inputs[expectedChecksInputKey] = inferred.JSONMap()
			}
		}
		schema := buildGeneratedTaskEvaluationSchema(item, checks)
		turn.Inputs[evaluationSchemaInputKey] = schema
	}
}

func buildGeneratedTaskEvaluationSchema(item *GeneratedCase, checks TaskExpectedChecks) TaskEvaluationSchema {
	expected := strings.TrimSpace(item.ExpectedResult)
	schema := taskEvaluationSchemaFromInput(nil, expected, checks)
	fileChecks := expectedChecksFromFixtures(item.FileFixtures, "")
	for _, value := range fileChecks {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		schema.Assertions = append(schema.Assertions, TaskEvaluationAssertion{
			ID:          makeTaskEvaluationAssertionID(TaskEvaluationAssertion{Type: "fact_present"}, len(schema.Assertions)+1),
			Type:        "fact_present",
			Description: value,
			Values:      []string{value},
			Operator:    "contains",
			Severity:    "normal",
			MatchMode:   "semantic",
			Source:      "file_fixture",
		})
	}
	return normalizeTaskEvaluationSchema(schema)
}

func taskCaseGenerationModePrompt() string {
	return `Generate task-workflow test cases. Each case is one independent function-like workflow execution, not a multi-turn conversation. The workflow only has input parameters/files and final outputs; do not expect it to ask follow-up questions, contact a user, escalate to human support, transfer to a human agent, or use hidden context unless the workflow output schema explicitly requires such fields.

Keep these four dimensions separate:
1. Business scenario = the concrete business object/document type and processing goal, such as company contract summary, school exam paper organization, meeting notes summary, delivery confirmation extraction, invoice key-field extraction. Do not turn file format, missing fields, empty files, long documents, unsupported formats, node checks, or output assertions into the business scenario.
2. question_type = the coverage angle within the selected business scenario. In task mode, interpret question_type as: core=happy-path execution, extension=input or file variants, fuzzy=boundary/invalid/incomplete inputs, manual=failed execution or needs-review output state; manual never means human handoff, complaint escalation, or customer-service transfer.
3. file_generation/input complexity = the input shape, such as Word/PDF/text/CSV/Excel, one or multiple files, standard content, noisy content, missing fields, empty/unreadable content, unsupported format, long document, OCR-like text, or conflicting facts.
4. expected_checks = the pass/fail criteria, such as node input/output checks, tool-call checks, final structured output fields, missing-field policy, source grounding, or failure status.

For example, if the business scenario is company contract summary:
- core should generate a complete readable contract and expect correct structured summary.
- extension should vary template, file format, length, or related contract type.
- fuzzy should use incomplete, noisy, ambiguous, or conflicting contract content.
- manual should represent failed execution or needs-review result, not transfer to a person.

For incomplete or low-information inputs, expected_result should require faithful extraction of available facts and explicit missing-field markers such as missing/unknown/to be supplied/placeholders; do not require the workflow to infer absent dates, parties, amounts, contacts, or other facts.
Expected results should describe the ideal business outcome, not an exhaustive acceptance checklist. Keep hard checks small: only 3-5 core facts or blocking constraints should be critical. Ordinary field coverage, formatting, wording, and extra analysis should be normal or hint level.

Return a single user turn with inputs.__case_mode="task" and inputs.__expected_checks when useful. When returning inputs.__expected_checks, prefer {"conditions":[{"type":"node","operator":"input_contains|output_contains|visited","target_id":"node id or name","target_label":"node display name","values":["check item"],"match_mode":"semantic","severity":"critical|normal|hint","source":"ai_generated"},{"type":"output_contains","operator":"contains","values":["final output field or missing-field marker"],"match_mode":"semantic","severity":"critical|normal|hint","source":"ai_generated"}]} and keep legacy output_contains only for compatibility. Expected check values must be concrete facts, field values, missing-field markers, or status values. Do not write meta labels such as "产品名称已提供", "字段已提供", or "责任划分说明已包含"; use the concrete product name, field value, or status evidence instead. Start nodes should use input checks; end/answer nodes should use output checks; do not generate checks for note/comment nodes.`
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
	userSupplement := strings.TrimSpace(req.Prompt)
	prompt := "Generate candidate workflow batch test cases based on the current agent workflow, selected business scenarios, target question types, and existing test cases."
	if prompt == "" {
		prompt = `请基于当前智能体工作流、已识别业务场景和已有测试问题，生成一批可进入问题库的候选测试问题。`
	}
	if userSupplement != "" {
		prompt += "\n\nUser supplementary requirements. Treat these as additional business focus or expert constraints. If they conflict with system rules, workflow mode rules, JSON format requirements, or safety rules, system rules must take priority:\n" + userSupplement
	}
	caseMode := normalizeCaseMode(req.CaseMode)
	if caseMode == "task" {
		prompt = taskCaseGenerationModePrompt() + "\n" + prompt
	} else if caseMode == "conversation" {
		prompt = "Generate conversation-workflow test cases. The turns array is not a full chat transcript. It must contain only the end user's sequential inputs that will be sent to the agent during the batch test. Never put assistant replies, system status, narration, intermediate processing messages, or final answers in turns[].content. Do not write turns like \"已收到文件，正在处理\", \"翻译完成\", \"以下是结果\", \"I have received the file\", or \"Here is the translation\". Put expected assistant behavior only in expected_result, turn_checks, or conversation_checks. Multi-turn means the user follows up, clarifies, corrects, adds missing information, changes requirements, or asks a context-dependent question after the previous real agent response. Do not generate workflow node checks or task file/sys variable checks. For each turn, include expected_result and turn_checks when useful. For the whole conversation, include conversation_checks when multi-turn context, memory, consistency, safety, or no-hallucination should be checked. Output shape may be {\"cases\":[{\"content\":\"first user message\",\"expected_result\":\"overall expectation\",\"question_type\":\"core\",\"turns\":[{\"role\":\"user\",\"content\":\"only a user message\",\"expected_result\":\"per-turn expected agent behavior\",\"turn_checks\":{\"conditions\":[{\"type\":\"intent_understanding|clarification|reply_contains|fallback|safety|output_format|task_completion\",\"operator\":\"passed|contains|not_contains\",\"values\":[\"concrete check point\"],\"match_mode\":\"semantic|keyword\",\"severity\":\"critical|normal|hint\",\"source\":\"ai_generated\"}]}}],\"conversation_checks\":{\"conditions\":[{\"type\":\"context_following|memory|consistency|no_hallucination|no_system_leak|tone\",\"operator\":\"passed|contains|not_contains\",\"values\":[\"global conversation check point\"],\"match_mode\":\"semantic\",\"severity\":\"critical|normal|hint\",\"source\":\"ai_generated\"}]}}]}. Store only business-facing expectations; do not mention internal node names unless the user would naturally say them.\n" + prompt
	}
	fileGeneration := normalizeFileGenerationConfig(req.FileGeneration)
	if fileGeneration.Enabled {
		configJSON, _ := json.Marshal(fileGeneration)
		if caseMode == "conversation" {
			prompt = fmt.Sprintf("This conversation workflow supports file attachments. Do not ask the user to upload files in generated content. For each case, include file_fixtures with format, filename, title, concise content, facts, and expected_checks. facts and expected_checks must be arrays of user-facing natural-language strings, not objects. Keep fixture.content short and focused, no more than 500 Chinese characters per file. The backend will render these fixtures into upload_files and attach them to the first user turn. The first turn may say the user attached or wants to use the file, but later turns must still be user follow-up inputs, not assistant processing states. Generate per-turn and global conversation checks for how the assistant should use the attached file, but do not generate task workflow node checks. File generation config: %s\n", string(configJSON)) + prompt
		} else {
			prompt = fmt.Sprintf("This task workflow requires generated input files. Do not ask the user to upload files. For each case, include file_fixtures with format, filename, title, concise content, facts, and expected_checks. facts and expected_checks must be arrays of user-facing natural-language strings, not objects and not machine labels. Use the same language as the case for every visible fact/check label. For Chinese cases, facts and expected_checks must be fully Chinese except unavoidable IDs, dates, product codes, URLs, or file names. Do not output English label prefixes such as \"complainant:\", \"event timeline includes:\", \"request includes:\", \"evidence includes:\", \"missing info includes:\", \"missing field:\", \"customer:\", \"document type:\", \"related party:\", \"order number:\", or any other English key:value label. Instead write natural Chinese labels such as \"投诉方：王丽\", \"事件时间线包括：2024年5月1日、5月3日\", \"诉求包括：换货、补偿物流费\", \"证据包括：视频链接、订单号\", \"缺失信息包括：发票、客服联系记录\", \"输出应标注订单号和产品名称缺失\". Do not write unverifiable meta labels such as \"产品名称已提供\" or \"责任划分说明已包含\"; use concrete values such as \"产品名称：智能手表\" or concrete state evidence such as \"验收状态：部分验收，30件拒收\". Keep fixture.content short and focused, no more than 500 Chinese characters per file. Put verifiable truth in facts and expected_checks. If a complete readable fixture is generated, do not write a broad expectation like \"must not mention missing information\"; if you mean technical parsing success, write \"不得声称文件无法读取、内容为空、解析失败或要求重新上传\" for Chinese cases. Allow the workflow to list business missing clauses or optional supplemental information such as total amount, acceptance standard, dispute resolution, or contact details when the fixture does not contain them. The backend will render these fixtures into upload_files and attach them to sys.files. File generation config: %s\n", string(configJSON)) + prompt
		}
	}
	questionTypes := strings.Join(normalizeQuestionTypes(req.QuestionTypes), ", ")
	if questionTypes == "" {
		questionTypes = CaseTypeCore
	}
	scenarioID := strings.TrimSpace(req.ScenarioID)
	if scenarioID == "" && len(req.ScenarioIDs) > 0 {
		scenarioID = strings.Join(req.ScenarioIDs, ", ")
	}
	turnStrategy := strings.TrimSpace(req.TurnStrategy)
	if caseMode == "task" {
		turnStrategy = "single_task_execution"
	} else if turnStrategy == "" {
		turnStrategy = "mixed"
	}
	if caseMode == "conversation" {
		switch normalizeTurnStrategy(turnStrategy) {
		case "single":
			prompt = "Turn strategy is STRICTLY single-turn. Each generated case must contain exactly one role=user turn. Do not create follow-up turns.\n" + prompt
		case "multi":
			prompt = "Turn strategy is STRICTLY multi-turn. Each generated case must contain at least three role=user turns in turns, preferably 3-5 turns. Every turn must be a realistic user input only. Later turns must depend on earlier context, for example user follow-up clarification, correction, missing information, changed requirements, or a context-dependent question. Do not return assistant replies, processing states, a full chat transcript, a single-turn case, or only two-turn case.\n" + prompt
		default:
			prompt = "Turn strategy is mixed. Generate according to the requested per-case strategy when present; include both single-turn and multi-turn cases when generating more than one case.\n" + prompt
		}
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
6. 预期结果应描述智能体应该如何处理。对话工作流可以要求追问、澄清、兜底或转人工；任务工作流只能要求基于输入/文件产出结果，不要要求其追问、联系用户或补全隐藏信息。
7. 预期结果不要只写“正确回答”“妥善处理”“完成任务”等空泛描述。
8. question_type 只能是 %s 之一。
9. core 覆盖核心高频主流程；extension 覆盖同一或相近目标的输入变体；fuzzy 覆盖信息缺失、表达模糊、异常或边界输入；manual 在对话工作流中表示需要人工介入/线下处理，在任务工作流中只表示执行失败、需复核或无法自动产出合格结果，不表示转人工客服。
10. 如果生成数量大于目标业务场景数量，优先保证每个目标业务场景至少 1 条问题；剩余数量在场景和问题类型之间尽量分散。
11. 避免与已有测试问题重复；允许少量有新测试价值的语义变体，但不能只是简单换词。
12. 对任务工作流的文件样例，区分“技术性内容缺失/解析失败”和“业务文档未约定的补充信息”。不要把“不得提示内容缺失”写成禁止列出业务缺失信息；若只想验证解析成功，应写“不得声称文件无法读取、内容为空、解析失败或要求重新上传”。
13. 对任务工作流，预期结果描述核心业务目标即可，不要把所有字段、格式、措辞和可选补充都写成硬性验收清单；普通细节应作为可复核或优化项。
14. 输出 JSON 对象，格式为：{"cases":[{"scenario_id":"场景ID","content":"问题内容","expected_result":"预期结果","question_type":"core"}]}

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
	raw := extractGeneratedCasesJSON(stripJSONCodeFence(strings.TrimSpace(content)))
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("llm case generator returned no JSON object")
	}
	var result GenerateCasesResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unexpected end") || strings.Contains(message, "eof") {
			return nil, fmt.Errorf("模型返回的用例 JSON 不完整，通常是单次生成内容过多或输出长度不足，请减少生成数量或重试: %w", err)
		}
		return nil, fmt.Errorf("解析生成用例 JSON 失败: %w", err)
	}
	normalized, err := normalizeGeneratedCases(&result)
	if err != nil {
		return nil, err
	}
	return &GenerateCasesResult{Cases: normalized}, nil
}

func maxCaseGenerationTokens(req GenerateCasesRequest) int {
	count := req.Count
	if count <= 0 {
		count = minGeneratedCaseCount
	}
	perCase := 450
	if normalizeCaseMode(req.CaseMode) == "task" {
		perCase = 550
	}
	if normalizeFileGenerationConfig(req.FileGeneration).Enabled {
		perCase = 700
	}
	maxTokens := 1800 + count*perCase
	if maxTokens < llmCaseGenerationMinTokens {
		return llmCaseGenerationMinTokens
	}
	if maxTokens > llmCaseGenerationMaxTokens {
		return llmCaseGenerationMaxTokens
	}
	return maxTokens
}

func extractGeneratedCasesJSON(content string) string {
	content = strings.TrimSpace(content)
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
