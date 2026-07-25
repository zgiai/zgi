package skillloop

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	skillToolArgumentsMaxJSONBytes = 1 << 20
	skillToolArgumentsMaxJSONDepth = 64

	skillToolArgumentsInvalidJSONCode = "skill_tool_arguments_invalid_json"
	skillToolArgumentsWrongTypeCode   = "skill_tool_arguments_wrong_type"
	skillToolArgumentsMissingCode     = "skill_tool_arguments_missing"
	skillToolRetryNoProgressCode      = "skill_tool_retry_no_progress"
)

type skillToolArgumentsError struct {
	Code              string
	SkillID           string
	ToolName          string
	ExpectedType      string
	ActualType        string
	MissingFields     []string
	ExpectedArguments map[string]interface{}
	RetryAction       string
	Cause             error
}

func (e *skillToolArgumentsError) Error() string {
	if e == nil {
		return ""
	}
	switch e.Code {
	case skillToolArgumentsInvalidJSONCode:
		return "skill tool arguments must be a complete JSON object"
	case skillToolArgumentsWrongTypeCode:
		return fmt.Sprintf("skill tool arguments must be an object, got %s", firstNonEmptyString(e.ActualType, "unknown"))
	case skillToolArgumentsMissingCode:
		return fmt.Sprintf("skill tool arguments are missing required field(s): %s", strings.Join(e.MissingFields, ", "))
	default:
		if e.Cause != nil {
			return e.Cause.Error()
		}
		return "skill tool arguments are invalid"
	}
}

func (e *skillToolArgumentsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func boolArg(args map[string]interface{}, key string) bool {
	if args == nil {
		return false
	}
	value, ok := args[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func normalizedSkillArg(args map[string]interface{}, key string) string {
	return strings.ToLower(stringArg(args, key))
}

func mapArg(args map[string]interface{}, key string) map[string]interface{} {
	if args == nil {
		return map[string]interface{}{}
	}
	value, ok := args[key]
	if !ok || value == nil {
		return map[string]interface{}{}
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return map[string]interface{}{}
}

func normalizeSkillToolArguments(args map[string]interface{}, skillID string, toolName string) (map[string]interface{}, error) {
	value, exists := args["arguments"]
	if !exists || value == nil {
		value = map[string]interface{}{}
	}

	var normalized map[string]interface{}
	switch typed := value.(type) {
	case map[string]interface{}:
		normalized = copyStringAnyMap(typed)
	case string:
		trimmed := strings.TrimSpace(typed)
		if len([]byte(trimmed)) > skillToolArgumentsMaxJSONBytes {
			return nil, newSkillToolArgumentsError(
				skillToolArgumentsInvalidJSONCode,
				skillID,
				toolName,
				"string",
				fmt.Errorf("arguments JSON exceeds %d bytes", skillToolArgumentsMaxJSONBytes),
			)
		}
		decoder := json.NewDecoder(bytes.NewBufferString(trimmed))
		var decoded interface{}
		if trimmed == "" || decoder.Decode(&decoded) != nil {
			return nil, newSkillToolArgumentsError(
				skillToolArgumentsInvalidJSONCode,
				skillID,
				toolName,
				"string",
				fmt.Errorf("arguments string is not valid JSON"),
			)
		}
		var trailing interface{}
		if err := decoder.Decode(&trailing); err != io.EOF {
			return nil, newSkillToolArgumentsError(
				skillToolArgumentsInvalidJSONCode,
				skillID,
				toolName,
				"string",
				fmt.Errorf("arguments string contains trailing JSON data"),
			)
		}
		object, ok := decoded.(map[string]interface{})
		if !ok {
			return nil, newSkillToolArgumentsError(
				skillToolArgumentsWrongTypeCode,
				skillID,
				toolName,
				jsonValueType(decoded),
				nil,
			)
		}
		if jsonValueDepth(object, 1) > skillToolArgumentsMaxJSONDepth {
			return nil, newSkillToolArgumentsError(
				skillToolArgumentsInvalidJSONCode,
				skillID,
				toolName,
				"string",
				fmt.Errorf("arguments JSON exceeds maximum depth"),
			)
		}
		normalized = object
	default:
		return nil, newSkillToolArgumentsError(
			skillToolArgumentsWrongTypeCode,
			skillID,
			toolName,
			jsonValueType(value),
			nil,
		)
	}

	if err := validateNormalizedSkillToolArgumentsBounds(normalized); err != nil {
		return nil, newSkillToolArgumentsError(
			skillToolArgumentsInvalidJSONCode,
			skillID,
			toolName,
			"object",
			err,
		)
	}
	missing := requiredSkillToolArgumentFields(skillID, toolName, normalized)
	if len(missing) > 0 {
		err := newSkillToolArgumentsError(
			skillToolArgumentsMissingCode,
			skillID,
			toolName,
			"object",
			nil,
		)
		err.MissingFields = missing
		err.RetryAction = "add the missing fields and retry call_skill_tool with an arguments object matching expected_arguments.schema"
		return nil, err
	}
	return normalized, nil
}

func validateNormalizedSkillToolArgumentsBounds(arguments map[string]interface{}) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return fmt.Errorf("arguments object is not JSON serializable: %w", err)
	}
	if len(encoded) > skillToolArgumentsMaxJSONBytes {
		return fmt.Errorf("arguments JSON exceeds %d bytes", skillToolArgumentsMaxJSONBytes)
	}
	if jsonValueDepth(arguments, 1) > skillToolArgumentsMaxJSONDepth {
		return fmt.Errorf("arguments JSON exceeds maximum depth")
	}
	return nil
}

func newSkillToolArgumentsError(code string, skillID string, toolName string, actualType string, cause error) *skillToolArgumentsError {
	retryAction := "retry call_skill_tool with arguments as a JSON object matching expected_arguments.schema"
	if code == skillToolArgumentsInvalidJSONCode {
		retryAction = "send one complete JSON object; do not send truncated JSON or an encoded non-object value"
	}
	return &skillToolArgumentsError{
		Code:              code,
		SkillID:           strings.ToLower(strings.TrimSpace(skillID)),
		ToolName:          strings.TrimSpace(toolName),
		ExpectedType:      "object",
		ActualType:        strings.TrimSpace(actualType),
		ExpectedArguments: skills.ExpectedSkillToolArguments(skillID, toolName),
		RetryAction:       retryAction,
		Cause:             cause,
	}
}

func requiredSkillToolArgumentFields(skillID string, toolName string, arguments map[string]interface{}) []string {
	expected := skills.ExpectedSkillToolArguments(skillID, toolName)
	schema, _ := expected["schema"].(map[string]interface{})
	rawRequired, ok := schema["required"]
	if !ok {
		return nil
	}
	required := make([]string, 0)
	switch typed := rawRequired.(type) {
	case []string:
		required = append(required, typed...)
	case []interface{}:
		for _, value := range typed {
			if field, ok := value.(string); ok && strings.TrimSpace(field) != "" {
				required = append(required, strings.TrimSpace(field))
			}
		}
	}
	missing := make([]string, 0, len(required))
	for _, field := range required {
		value, exists := arguments[field]
		if !exists || !skillToolArgumentValuePresent(value) {
			missing = append(missing, field)
		}
	}
	return missing
}

func skillToolArgumentValuePresent(value interface{}) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func jsonValueType(value interface{}) string {
	if value == nil {
		return "null"
	}
	switch value.(type) {
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return reflect.TypeOf(value).String()
	}
}

func jsonValueDepth(value interface{}, depth int) int {
	maxDepth := depth
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, child := range typed {
			if childDepth := jsonValueDepth(child, depth+1); childDepth > maxDepth {
				maxDepth = childDepth
			}
		}
	case []interface{}:
		for _, child := range typed {
			if childDepth := jsonValueDepth(child, depth+1); childDepth > maxDepth {
				maxDepth = childDepth
			}
		}
	}
	return maxDepth
}

func rawSkillToolArgumentsFingerprint(value interface{}) map[string]interface{} {
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded = []byte(fmt.Sprint(value))
	}
	return map[string]interface{}{
		"actual_type": jsonValueType(value),
		"digest":      fmt.Sprintf("%x", sha256Bytes(encoded)),
	}
}

func sha256Bytes(value []byte) [32]byte {
	return sha256.Sum256(value)
}

func partialJSONStringField(input string, field string) (string, bool) {
	start, ok := findJSONStringFieldValueStart(input, field)
	if !ok {
		return "", false
	}
	value, _, complete := decodePartialJSONString(input[start:])
	return value, complete
}

func findJSONStringFieldValueStart(input string, field string) (int, bool) {
	for i := 0; i < len(input); i++ {
		if input[i] != '"' {
			continue
		}
		keyStart := i
		key, keyEnd, complete := decodeJSONStringToken(input, keyStart)
		if !complete || key != field {
			continue
		}
		j := skipJSONWhitespace(input, keyEnd)
		if j >= len(input) || input[j] != ':' {
			continue
		}
		j = skipJSONWhitespace(input, j+1)
		if j < len(input) && input[j] == '"' {
			return j + 1, true
		}
	}
	return 0, false
}

func decodeJSONStringToken(input string, quoteStart int) (string, int, bool) {
	if quoteStart < 0 || quoteStart >= len(input) || input[quoteStart] != '"' {
		return "", quoteStart, false
	}
	value, consumed, complete := decodePartialJSONString(input[quoteStart+1:])
	return value, quoteStart + 1 + consumed, complete
}

func decodePartialJSONString(input string) (string, int, bool) {
	var builder strings.Builder
	for i := 0; i < len(input); i++ {
		ch := input[i]
		switch ch {
		case '"':
			return builder.String(), i + 1, true
		case '\\':
			if i+1 >= len(input) {
				return builder.String(), i, false
			}
			next := input[i+1]
			switch next {
			case '"', '\\', '/':
				builder.WriteByte(next)
				i++
			case 'b':
				builder.WriteByte('\b')
				i++
			case 'f':
				builder.WriteByte('\f')
				i++
			case 'n':
				builder.WriteByte('\n')
				i++
			case 'r':
				builder.WriteByte('\r')
				i++
			case 't':
				builder.WriteByte('\t')
				i++
			case 'u':
				if i+6 > len(input) {
					return builder.String(), i, false
				}
				value, err := strconv.ParseInt(input[i+2:i+6], 16, 32)
				if err != nil {
					return builder.String(), i, false
				}
				builder.WriteRune(rune(value))
				i += 5
			default:
				return builder.String(), i, false
			}
		default:
			builder.WriteByte(ch)
		}
	}
	return builder.String(), len(input), false
}

func skipJSONWhitespace(input string, index int) int {
	for index < len(input) {
		switch input[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}
