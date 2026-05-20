package sqlgenerator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

var placeholderPattern = regexp.MustCompile(`\{\{(#[a-zA-Z0-9_]{1,50}(?:\.[a-zA-Z_][a-zA-Z0-9_]{0,29}){1,10}#)\}\}`)

type placeholder struct {
	token         string
	valueSelector []string
}

func extractPlaceholders(tpl string) []placeholder {
	matches := placeholderPattern.FindAllStringSubmatch(tpl, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(matches))
	result := make([]placeholder, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		token := match[1]
		if seen[token] {
			continue
		}
		seen[token] = true
		trimmed := strings.Trim(token, "#")
		parts := strings.Split(trimmed, ".")
		if len(parts) < 2 {
			continue
		}
		result = append(result, placeholder{
			token:         token,
			valueSelector: parts,
		})
	}
	return result
}

func renderTemplate(tpl string, values map[string]string) string {
	if len(values) == 0 {
		return tpl
	}
	return placeholderPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		submatches := placeholderPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		token := submatches[1]
		if val, ok := values[token]; ok {
			return val
		}
		return ""
	})
}

func resolveTemplate(tpl string, vp *entities.VariablePool) (string, map[string]any, error) {
	holders := extractPlaceholders(tpl)
	if len(holders) == 0 {
		return tpl, map[string]any{}, nil
	}

	textValues := make(map[string]string, len(holders))
	resolved := make(map[string]any, len(holders))

	for _, holder := range holders {
		variable := vp.GetWithPath(holder.valueSelector)
		if variable == nil {
			return "", nil, fmt.Errorf("variable %s not found", strings.Trim(holder.token, "#"))
		}

		obj := variable.ToObject()
		key := strings.Trim(holder.token, "#")
		resolved[key] = obj
		textValues[holder.token] = formatAny(obj)
	}

	return renderTemplate(tpl, textValues), resolved, nil
}

func formatAny(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	case []byte:
		return string(val)
	case map[string]any, []any:
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func deriveVariableSelectorsFromPrompt(prompt string) []VariableSelector {
	holders := extractPlaceholders(prompt)
	if len(holders) == 0 {
		return nil
	}

	result := make([]VariableSelector, 0, len(holders))
	seen := make(map[string]struct{}, len(holders))
	for _, holder := range holders {
		if len(holder.valueSelector) < 2 {
			continue
		}
		key := strings.Join(holder.valueSelector, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, VariableSelector{
			Variable:      holder.token,
			ValueSelector: holder.valueSelector,
		})
	}
	return result
}

func resolveQuickBindings(bindings []VariableSelector, vp *entities.VariablePool) map[string]any {
	result := make(map[string]any, len(bindings))
	for _, binding := range bindings {
		if len(binding.ValueSelector) < 2 {
			continue
		}
		if vp == nil {
			continue
		}
		variable := vp.GetWithPath(binding.ValueSelector)
		if variable == nil {
			continue
		}
		key := strings.Join(binding.ValueSelector, ".")
		result[key] = variable.ToObject()
	}
	return result
}
