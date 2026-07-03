package suggestedquestions

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type modelResponse struct {
	Questions          json.RawMessage `json:"questions"`
	SuggestedQuestions json.RawMessage `json:"suggested_questions"`
	Items              json.RawMessage `json:"items"`
	Candidates         json.RawMessage `json:"candidates"`
	Warnings           []string        `json:"warnings"`
}

var (
	thinkingBlockPattern = regexp.MustCompile(`(?is)<think(?:ing)?\b[^>]*>.*?</think(?:ing)?>`)
	listQuestionPattern  = regexp.MustCompile(`^\s*(?:[-*•]\s+|\d+[\.)、]\s+|[（(]\d+[）)]\s*)(.+)$`)
)

// ParseQuestions parses model output and returns normalized question candidates.
func ParseQuestions(raw string, count int, existing []string) ([]Question, []string, error) {
	cleanedRaw := stripThinkingContent(strings.TrimSpace(raw))
	if cleanedRaw == "" {
		return nil, nil, fmt.Errorf("empty model response")
	}

	var lastErr error
	for _, candidate := range extractJSONCandidates(cleanedRaw) {
		questions, warnings, err := parseJSONQuestions(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		normalized := normalizeQuestions(questions, count, existing)
		if len(normalized) > 0 {
			return normalized, uniqueTrimmed(warnings, 6), nil
		}
	}

	if questions := parseListQuestions(cleanedRaw); len(questions) > 0 {
		return normalizeQuestions(questions, count, existing), nil, nil
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("response did not contain suggested questions")
}

func parseJSONQuestions(raw string) ([]Question, []string, error) {
	var response modelResponse
	if err := json.Unmarshal([]byte(raw), &response); err == nil {
		for _, payload := range []json.RawMessage{
			response.Questions,
			response.SuggestedQuestions,
			response.Items,
			response.Candidates,
		} {
			if len(payload) == 0 {
				continue
			}
			if questions := parseQuestionValue(payload); len(questions) > 0 {
				return questions, response.Warnings, nil
			}
		}
	}

	questions := parseQuestionValue([]byte(raw))
	if len(questions) == 0 {
		return nil, nil, fmt.Errorf("response did not contain suggested questions")
	}
	return questions, nil, nil
}

func parseQuestionValue(raw []byte) []Question {
	var items []interface{}
	if err := json.Unmarshal(raw, &items); err == nil {
		out := make([]Question, 0, len(items))
		for _, item := range items {
			if question, ok := questionFromAny(item); ok {
				out = append(out, question)
			}
		}
		return out
	}

	var item interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil
	}
	if question, ok := questionFromAny(item); ok {
		return []Question{question}
	}
	return nil
}

func questionFromAny(value interface{}) (Question, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return Question{}, false
		}
		return Question{Text: text}, true
	case map[string]interface{}:
		text := firstQuestionString(typed, "text", "question", "content", "title", "query", "input")
		if text == "" {
			return Question{}, false
		}
		return Question{
			Text:   text,
			Reason: firstQuestionString(typed, "reason", "description", "rationale"),
		}, true
	default:
		return Question{}, false
	}
}

func firstQuestionString(record map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := record[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseListQuestions(raw string) []Question {
	lines := strings.Split(raw, "\n")
	out := make([]Question, 0, len(lines))
	for _, line := range lines {
		match := listQuestionPattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		text := strings.Trim(strings.TrimSpace(match[1]), "`\"'“”")
		if text == "" {
			continue
		}
		out = append(out, Question{Text: text})
	}
	return out
}

func normalizeQuestions(questions []Question, count int, existing []string) []Question {
	if count <= 0 {
		count = defaultQuestionCount
	}
	if count > maxQuestionCount {
		count = maxQuestionCount
	}

	seen := make(map[string]struct{}, len(existing)+len(questions))
	for _, item := range existing {
		text := strings.ToLower(cleanShortText(item))
		if text != "" {
			seen[text] = struct{}{}
		}
	}

	result := make([]Question, 0, count)
	for _, item := range questions {
		text := cleanText(item.Text, 120)
		if text == "" {
			continue
		}
		key := strings.ToLower(text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, Question{
			Text:   text,
			Reason: cleanText(item.Reason, 180),
		})
		if len(result) >= count {
			break
		}
	}
	return result
}

func stripThinkingContent(raw string) string {
	return strings.TrimSpace(thinkingBlockPattern.ReplaceAllString(raw, ""))
}

func extractJSONCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```JSON")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	if json.Valid([]byte(raw)) && (strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[")) {
		return []string{raw}
	}

	candidates := make([]string, 0, 2)
	for start, char := range raw {
		if char != '{' && char != '[' {
			continue
		}
		closing := byte('}')
		if char == '[' {
			closing = ']'
		}
		for end := len(raw) - 1; end > start; end-- {
			if raw[end] != closing {
				continue
			}
			candidate := strings.TrimSpace(raw[start : end+1])
			if json.Valid([]byte(candidate)) {
				candidates = append(candidates, candidate)
				break
			}
		}
	}
	return candidates
}
