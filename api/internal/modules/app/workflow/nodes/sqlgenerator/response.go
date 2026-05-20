package sqlgenerator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type generationOutput struct {
	SQL        string
	Analysis   string
	UsedFields []string
	RawJSON    map[string]any
}

var codeBlockPattern = regexp.MustCompile("(?s)```(?P<lang>[a-zA-Z0-9]*)\\s*(?P<body>.+?)```")

func parseLLMContent(raw string) (generationOutput, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return generationOutput{}, fmt.Errorf("empty LLM response")
	}

	if out, ok := tryParseFromJSON(text); ok {
		return out, nil
	}

	if block, lang, ok := extractCodeBlock(text); ok {
		if strings.EqualFold(lang, "json") {
			if out, ok := tryParseFromJSON(block); ok {
				return out, nil
			}
		}
		if strings.EqualFold(lang, "sql") || lang == "" {
			sqlText := strings.TrimSpace(block)
			if sqlText != "" {
				return generationOutput{SQL: sqlText}, nil
			}
		}
	}

	if out := tryParseInlineJSON(text); out.SQL != "" || len(out.RawJSON) > 0 {
		return out, nil
	}

	if sqlText := extractSQLFromText(text); sqlText != "" {
		return generationOutput{SQL: sqlText}, nil
	}

	return generationOutput{}, fmt.Errorf("unable to extract SQL from LLM response")
}

func tryParseFromJSON(text string) (generationOutput, bool) {
	candidate := strings.TrimSpace(text)
	if strings.HasPrefix(candidate, "```") {
		if block, lang, ok := extractCodeBlock(candidate); ok {
			if strings.EqualFold(lang, "json") {
				candidate = block
			}
		}
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(candidate), &payload); err != nil {
		return generationOutput{}, false
	}
	return extractFieldsFromPayload(payload), true
}

func tryParseInlineJSON(text string) generationOutput {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return generationOutput{}
	}

	inline := text[start : end+1]
	var payload map[string]any
	if err := json.Unmarshal([]byte(inline), &payload); err != nil {
		return generationOutput{}
	}
	return extractFieldsFromPayload(payload)
}

func extractFieldsFromPayload(payload map[string]any) generationOutput {
	getString := func(keys ...string) string {
		for _, key := range keys {
			if val, ok := payload[key]; ok {
				switch t := val.(type) {
				case string:
					if strings.TrimSpace(t) != "" {
						return strings.TrimSpace(t)
					}
				}
			}
		}
		return ""
	}

	sqlText := getString("sql", "query", "statement")
	result := generationOutput{
		SQL:      sqlText,
		Analysis: getString("analysis", "explanation", "comment"),
		RawJSON:  payload,
	}

	if fieldsVal, ok := payload["used_fields"]; ok {
		result.UsedFields = convertToStringSlice(fieldsVal)
	} else if fieldsVal, ok := payload["fields"]; ok {
		result.UsedFields = convertToStringSlice(fieldsVal)
	} else if fieldsVal, ok := payload["columns"]; ok {
		result.UsedFields = convertToStringSlice(fieldsVal)
	}

	return result
}

func convertToStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				if strings.TrimSpace(str) != "" {
					out = append(out, strings.TrimSpace(str))
				}
			}
		}
		return out
	default:
		return nil
	}
}

func extractCodeBlock(text string) (string, string, bool) {
	matches := codeBlockPattern.FindStringSubmatch(text)
	if len(matches) < 3 {
		return "", "", false
	}
	return strings.TrimSpace(matches[2]), strings.TrimSpace(matches[1]), true
}

func extractSQLFromText(text string) string {
	lines := strings.Split(text, "\n")
	builder := strings.Builder{}
	foundSQL := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Remove markdown quote prefix (>)
		if strings.HasPrefix(trimmed, ">") {
			trimmed = strings.TrimSpace(trimmed[1:])
		}

		// Handle "sql:" prefix
		if strings.HasPrefix(strings.ToLower(trimmed), "sql:") {
			trimmed = strings.TrimSpace(trimmed[4:])
		}

		// Check if this line looks like SQL
		lowerLine := strings.ToLower(trimmed)
		if strings.HasPrefix(lowerLine, "select") ||
			strings.HasPrefix(lowerLine, "with") ||
			strings.HasPrefix(lowerLine, "insert") ||
			strings.HasPrefix(lowerLine, "update") ||
			strings.HasPrefix(lowerLine, "delete") ||
			strings.HasPrefix(lowerLine, "create") ||
			strings.HasPrefix(lowerLine, "alter") ||
			strings.HasPrefix(lowerLine, "drop") {
			foundSQL = true
		}

		// If we found SQL, start collecting
		if foundSQL {
			builder.WriteString(trimmed)
			builder.WriteString("\n")

			// Stop if we hit a semicolon (end of statement)
			if strings.HasSuffix(trimmed, ";") {
				break
			}
		}
	}

	sqlCandidate := strings.TrimSpace(builder.String())
	if sqlCandidate == "" {
		return ""
	}

	// Final validation: must start with a SQL keyword
	lower := strings.ToLower(sqlCandidate)
	if strings.HasPrefix(lower, "select") ||
		strings.HasPrefix(lower, "with") ||
		strings.HasPrefix(lower, "insert") ||
		strings.HasPrefix(lower, "update") ||
		strings.HasPrefix(lower, "delete") ||
		strings.HasPrefix(lower, "create") ||
		strings.HasPrefix(lower, "alter") ||
		strings.HasPrefix(lower, "drop") {
		return sqlCandidate
	}
	return ""
}
