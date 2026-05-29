package service

import (
	"strconv"
	"strings"
)

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
