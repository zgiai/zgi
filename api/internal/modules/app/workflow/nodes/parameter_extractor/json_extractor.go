package parameterextractor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSONExtractor handles extraction and parsing of JSON from LLM responses
type JSONExtractor struct{}

// NewJSONExtractor creates a new JSONExtractor instance
func NewJSONExtractor() *JSONExtractor {
	return &JSONExtractor{}
}

// ExtractFromText extracts JSON from plain text response
// This handles prompt engineering mode where JSON is embedded in text
func (je *JSONExtractor) ExtractFromText(text string) (map[string]any, error) {
	if text == "" {
		return nil, NewInvalidInvokeResultError("text is empty")
	}

	// Extract complete JSON string using stack-based bracket matching
	jsonStr, err := je.extractCompleteJSON(text)
	if err != nil {
		return nil, NewInvalidInvokeResultError(fmt.Sprintf("failed to extract JSON: %v", err))
	}

	// Parse the extracted JSON string
	var result map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, NewInvalidInvokeResultError(fmt.Sprintf("failed to parse JSON: %v", err))
	}

	return result, nil
}

// extractCompleteJSON extracts the first complete JSON object or array from text
// Uses a stack-based algorithm to handle nested structures correctly
func (je *JSONExtractor) extractCompleteJSON(text string) (string, error) {
	// Find the first opening bracket (object or array)
	startIdx := -1
	startChar := rune(0)

	for i, c := range text {
		if c == '{' || c == '[' {
			startIdx = i
			startChar = c
			break
		}
	}

	if startIdx == -1 {
		return "", fmt.Errorf("no JSON object or array found in text")
	}

	// Use stack to match brackets and find the complete JSON
	stack := []rune{startChar}

	for i := startIdx + 1; i < len(text); i++ {
		c := rune(text[i])

		// Skip characters inside strings to avoid false matches
		if c == '"' {
			// Find the end of the string, handling escaped quotes
			i++
			for i < len(text) {
				if text[i] == '\\' {
					// Skip escaped character
					i++
				} else if text[i] == '"' {
					// End of string found
					break
				}
				i++
			}
			continue
		}

		// Handle opening brackets
		if c == '{' || c == '[' {
			stack = append(stack, c)
			continue
		}

		// Handle closing brackets
		if c == '}' || c == ']' {
			if len(stack) == 0 {
				return "", fmt.Errorf("unmatched closing bracket at position %d", i)
			}

			// Check if brackets match
			top := stack[len(stack)-1]
			if (c == '}' && top == '{') || (c == ']' && top == '[') {
				// Pop from stack
				stack = stack[:len(stack)-1]

				// If stack is empty, we found the complete JSON
				if len(stack) == 0 {
					return text[startIdx : i+1], nil
				}
			} else {
				return "", fmt.Errorf("mismatched brackets at position %d", i)
			}
		}
	}

	// If we reach here, brackets were not properly closed
	return "", fmt.Errorf("incomplete JSON: unclosed brackets")
}

// TryExtractJSON attempts to extract JSON from various response formats
// This is a convenience method that tries multiple extraction strategies
func (je *JSONExtractor) TryExtractJSON(response any) (map[string]any, error) {
	switch v := response.(type) {
	case string:
		return je.ExtractFromText(v)
	case map[string]any:
		// Already a map, return as-is
		return v, nil
	default:
		return nil, NewInvalidInvokeResultError(fmt.Sprintf("unsupported response type: %T", response))
	}
}

// ValidateJSON checks if a string contains valid JSON
func (je *JSONExtractor) ValidateJSON(text string) bool {
	var result map[string]any
	return json.Unmarshal([]byte(text), &result) == nil
}

// ExtractMultipleJSON extracts all JSON objects from text
// Useful when the LLM returns multiple JSON objects
func (je *JSONExtractor) ExtractMultipleJSON(text string) ([]map[string]any, error) {
	results := []map[string]any{}
	remaining := text

	for {
		// Try to extract JSON from remaining text
		jsonStr, err := je.extractCompleteJSON(remaining)
		if err != nil {
			// No more JSON found
			break
		}

		// Parse the extracted JSON
		var result map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			return nil, NewInvalidInvokeResultError(fmt.Sprintf("failed to parse JSON: %v", err))
		}

		results = append(results, result)

		// Find where this JSON ends in the remaining text
		idx := strings.Index(remaining, jsonStr)
		if idx == -1 {
			break
		}

		// Continue searching after this JSON
		remaining = remaining[idx+len(jsonStr):]
	}

	if len(results) == 0 {
		return nil, NewInvalidInvokeResultError("no valid JSON found in text")
	}

	return results, nil
}
