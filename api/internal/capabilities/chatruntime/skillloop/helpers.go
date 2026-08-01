package skillloop

import (
	"encoding/json"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const maxInvalidToolArgumentPreviewRunes = 800

var modelInvocationTokenEstimator = tokenestimate.NewEstimator()

func cloneChatRequest(source *adapter.ChatRequest) *adapter.ChatRequest {
	if source == nil {
		return &adapter.ChatRequest{}
	}
	cloned := *source
	cloned.Messages = cloneMessagesForProvider(source.Messages)
	cloned.Stop = append([]string{}, source.Stop...)
	if source.AdditionalParameters != nil {
		cloned.AdditionalParameters = copyStringAnyMap(source.AdditionalParameters)
	}
	if source.LogitBias != nil {
		cloned.LogitBias = make(map[string]float64, len(source.LogitBias))
		for key, value := range source.LogitBias {
			cloned.LogitBias[key] = value
		}
	}
	return &cloned
}

func chatRequestPromptChars(request *adapter.ChatRequest) int {
	if request == nil {
		return 0
	}
	payload := struct {
		Messages []adapter.Message `json:"messages"`
		Tools    []adapter.Tool    `json:"tools,omitempty"`
	}{
		Messages: request.Messages,
		Tools:    request.Tools,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return len([]rune(string(encoded)))
}

func chatRequestPromptEstimate(request *adapter.ChatRequest) tokenestimate.ChatRequestResult {
	return modelInvocationTokenEstimator.EstimateChatRequest(request)
}

func cloneMessagesForProvider(source []adapter.Message) []adapter.Message {
	if len(source) == 0 {
		return nil
	}
	out := make([]adapter.Message, 0, len(source))
	for _, message := range source {
		cloned := message
		cloned.ToolCalls = normalizeToolCalls(message.ToolCalls)
		if strings.EqualFold(strings.TrimSpace(cloned.Role), "tool") {
			cloned.Content = providerSafeToolContent(cloned.Content)
		}
		out = append(out, cloned)
	}
	return out
}

func providerSafeToolContent(content interface{}) interface{} {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(typed)
	}
}

func copyStringAnyMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func planningRespUsage(resp *adapter.ChatResponse) *adapter.Usage {
	if resp == nil {
		return nil
	}
	return resp.Usage
}

func mergeUsage(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		cloned := *next
		return &cloned
	}
	current.PromptTokens += next.PromptTokens
	current.CompletionTokens += next.CompletionTokens
	current.TotalTokens += next.TotalTokens
	return current
}

// mergeStreamUsageSnapshot merges cumulative usage snapshots emitted during one
// streaming model invocation. Providers may attach the latest usage to multiple
// chunks (including the terminal chunk), so summing those values inflates usage.
func mergeStreamUsageSnapshot(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		cloned := *next
		normalizeUsageTotal(&cloned)
		return &cloned
	}
	if next.PromptTokens > current.PromptTokens {
		current.PromptTokens = next.PromptTokens
	}
	if next.CompletionTokens > current.CompletionTokens {
		current.CompletionTokens = next.CompletionTokens
	}
	if next.TotalTokens > current.TotalTokens {
		current.TotalTokens = next.TotalTokens
	}
	normalizeUsageTotal(current)
	return current
}

func normalizeUsageTotal(usage *adapter.Usage) {
	if usage == nil {
		return
	}
	if componentTotal := usage.PromptTokens + usage.CompletionTokens; componentTotal > usage.TotalTokens {
		usage.TotalTokens = componentTotal
	}
}

func firstPlanningMessage(resp *adapter.ChatResponse) adapter.Message {
	if resp == nil || len(resp.Choices) == 0 {
		return adapter.Message{Role: "assistant"}
	}
	message := resp.Choices[0].Message
	if strings.TrimSpace(message.Role) == "" {
		message.Role = "assistant"
	}
	return message
}

func assistantMessageText(message adapter.Message) string {
	switch typed := message.Content.(type) {
	case string:
		return typed
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func normalizeToolCalls(calls []adapter.ToolCall) []adapter.ToolCall {
	out := make([]adapter.ToolCall, 0, len(calls))
	for idx, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("call_%d", idx+1)
		}
		if strings.TrimSpace(call.Type) == "" {
			call.Type = "function"
		}
		index := idx
		if call.Index == nil {
			call.Index = &index
		}
		if strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolFinalAnswer) {
			call.Function.Arguments = strings.TrimSpace(call.Function.Arguments)
			if call.Function.Arguments == "" {
				call.Function.Arguments = "{}"
			}
		} else {
			call.Function.Arguments = normalizeToolCallArguments(call.Function.Arguments)
		}
		out = append(out, call)
	}
	return out
}

func normalizeToolCallArguments(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	if normalized, ok := normalizeJSONObjectString(raw); ok {
		return normalized
	}
	repaired, changed := repairCommonJSONStringIssues(raw)
	if changed {
		if normalized, ok := normalizeJSONObjectString(repaired); ok {
			return normalized
		}
	}
	if closed, ok := closeOpenJSONContainers(repaired); ok {
		if normalized, normalizedOK := normalizeJSONObjectString(closed); normalizedOK {
			return normalized
		}
	}
	payload := map[string]interface{}{
		"_invalid_tool_arguments": true,
		"error":                   "tool arguments were not valid JSON",
		"raw_preview":             truncateRunesForHelper(raw, maxInvalidToolArgumentPreviewRunes),
		"next_action":             "retry the same tool with valid JSON arguments",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"_invalid_tool_arguments":true,"error":"tool arguments were not valid JSON"}`
	}
	return string(encoded)
}

func normalizeJSONObjectString(raw string) (string, bool) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return "", false
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func repairCommonJSONStringIssues(raw string) (string, bool) {
	var builder strings.Builder
	builder.Grow(len(raw) + 8)
	inString := false
	escaped := false
	changed := false
	for idx := 0; idx < len(raw); idx++ {
		ch := raw[idx]
		if !inString {
			builder.WriteByte(ch)
			if ch == '"' {
				inString = true
			}
			continue
		}
		if escaped {
			builder.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			if validJSONEscapeAt(raw, idx) {
				builder.WriteByte(ch)
				escaped = true
			} else {
				builder.WriteString(`\\`)
				changed = true
			}
			continue
		}
		if ch == '"' {
			if quoteLooksLikeJSONStringTerminator(raw, idx) {
				builder.WriteByte(ch)
				inString = false
				continue
			}
			builder.WriteByte('\\')
			builder.WriteByte('"')
			changed = true
			continue
		}
		switch ch {
		case '\n':
			builder.WriteString(`\n`)
			changed = true
			continue
		case '\r':
			builder.WriteString(`\r`)
			changed = true
			continue
		case '\t':
			builder.WriteString(`\t`)
			changed = true
			continue
		}
		if ch < 0x20 {
			const hex = "0123456789abcdef"
			builder.WriteString(`\u00`)
			builder.WriteByte(hex[ch>>4])
			builder.WriteByte(hex[ch&0x0f])
			changed = true
			continue
		}
		builder.WriteByte(ch)
	}
	if !changed {
		return raw, false
	}
	return builder.String(), true
}

func quoteLooksLikeJSONStringTerminator(raw string, quoteIndex int) bool {
	next := nextNonJSONWhitespace(raw, quoteIndex+1)
	if next >= len(raw) {
		return true
	}
	switch raw[next] {
	case '}', ']':
		return true
	case ':', ',':
		valueStart := nextNonJSONWhitespace(raw, next+1)
		return valueStart >= len(raw) || jsonValueCanStart(raw, valueStart)
	default:
		return false
	}
}

func nextNonJSONWhitespace(raw string, start int) int {
	for start < len(raw) {
		switch raw[start] {
		case ' ', '\t', '\r', '\n':
			start++
		default:
			return start
		}
	}
	return start
}

func jsonValueCanStart(raw string, start int) bool {
	if start < 0 || start >= len(raw) {
		return false
	}
	switch raw[start] {
	case '"', '{', '[', '-':
		return true
	case 't':
		return validJSONLiteralAt(raw, start, "true")
	case 'f':
		return validJSONLiteralAt(raw, start, "false")
	case 'n':
		return validJSONLiteralAt(raw, start, "null")
	default:
		return raw[start] >= '0' && raw[start] <= '9'
	}
}

func validJSONLiteralAt(raw string, start int, literal string) bool {
	end := start + len(literal)
	if end > len(raw) || raw[start:end] != literal {
		return false
	}
	if end == len(raw) {
		return true
	}
	switch raw[end] {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}

func validJSONEscapeAt(raw string, slashIndex int) bool {
	if slashIndex+1 >= len(raw) {
		return false
	}
	switch raw[slashIndex+1] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return true
	case 'u':
		if slashIndex+6 > len(raw) {
			return false
		}
		for idx := slashIndex + 2; idx < slashIndex+6; idx++ {
			if !isJSONHexDigit(raw[idx]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isJSONHexDigit(ch byte) bool {
	return ch >= '0' && ch <= '9' ||
		ch >= 'a' && ch <= 'f' ||
		ch >= 'A' && ch <= 'F'
}

func closeOpenJSONContainers(raw string) (string, bool) {
	stack := make([]byte, 0, 4)
	inString := false
	escaped := false
	for idx := 0; idx < len(raw); idx++ {
		ch := raw[idx]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			if len(stack) == 0 || !jsonContainerMatches(stack[len(stack)-1], ch) {
				return raw, false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inString || escaped || len(stack) == 0 {
		return raw, false
	}
	var builder strings.Builder
	builder.Grow(len(raw) + len(stack))
	builder.WriteString(raw)
	for idx := len(stack) - 1; idx >= 0; idx-- {
		if stack[idx] == '{' {
			builder.WriteByte('}')
		} else {
			builder.WriteByte(']')
		}
	}
	return builder.String(), true
}

func jsonContainerMatches(open byte, close byte) bool {
	return open == '{' && close == '}' || open == '[' && close == ']'
}

func truncateRunesForHelper(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func maxBusinessToolCalls(resolved *skills.ResolvedSkills) int {
	if resolved == nil || len(resolved.Skills) == 0 {
		return defaultMaxBusinessToolCallsPerSkill
	}
	total := 0
	for _, doc := range resolved.Skills {
		if doc.Metadata.MaxCallsPerTurn <= 0 {
			total += defaultMaxBusinessToolCallsPerSkill
			continue
		}
		total += doc.Metadata.MaxCallsPerTurn
	}
	if total <= 0 {
		return defaultMaxBusinessToolCallsPerSkill
	}
	return total
}

func maxBusinessToolCallsForSkill(resolved *skills.ResolvedSkills, skillID string) int {
	if resolved == nil {
		return defaultMaxBusinessToolCallsPerSkill
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	for _, doc := range resolved.Skills {
		if strings.ToLower(strings.TrimSpace(doc.Metadata.ID)) != skillID {
			continue
		}
		if doc.Metadata.MaxCallsPerTurn > 0 {
			return doc.Metadata.MaxCallsPerTurn
		}
		return defaultMaxBusinessToolCallsPerSkill
	}
	return defaultMaxBusinessToolCallsPerSkill
}

func incrementSkillToolCallCount(counts map[string]int, skillID string) {
	if counts == nil {
		return
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == "" {
		return
	}
	counts[skillID]++
}

func maxSkillStepsForTurn(resolved *skills.ResolvedSkills) int {
	limit := maxBusinessToolCalls(resolved)
	if resolved != nil {
		limit += len(resolved.Skills) * 2
	}
	if limit < defaultMaxSkillStepsPerTurn {
		return defaultMaxSkillStepsPerTurn
	}
	return limit
}
