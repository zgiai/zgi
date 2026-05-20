package suggestedquestions

import (
	"encoding/json"
	"fmt"
	"strings"
)

type modelResponse struct {
	Questions []Question `json:"questions"`
	Warnings  []string   `json:"warnings"`
}

// ParseQuestions parses model output and returns normalized question candidates.
func ParseQuestions(raw string, count int, existing []string) ([]Question, []string, error) {
	cleaned := extractJSON(strings.TrimSpace(raw))
	if cleaned == "" {
		return nil, nil, fmt.Errorf("empty model response")
	}

	var response modelResponse
	if err := json.Unmarshal([]byte(cleaned), &response); err != nil {
		var questions []Question
		if arrayErr := json.Unmarshal([]byte(cleaned), &questions); arrayErr == nil {
			response.Questions = questions
		} else {
			var stringsOnly []string
			if stringsErr := json.Unmarshal([]byte(cleaned), &stringsOnly); stringsErr != nil {
				return nil, nil, err
			}
			for _, item := range stringsOnly {
				response.Questions = append(response.Questions, Question{Text: item})
			}
		}
	}

	return normalizeQuestions(response.Questions, count, existing), uniqueTrimmed(response.Warnings, 6), nil
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

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		return raw
	}

	objStart := strings.Index(raw, "{")
	objEnd := strings.LastIndex(raw, "}")
	if objStart >= 0 && objEnd > objStart {
		return raw[objStart : objEnd+1]
	}

	arrayStart := strings.Index(raw, "[")
	arrayEnd := strings.LastIndex(raw, "]")
	if arrayStart >= 0 && arrayEnd > arrayStart {
		return raw[arrayStart : arrayEnd+1]
	}

	return ""
}
