package sensitiveredaction

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

// RedactTextTool detects and redacts sensitive text.
type RedactTextTool struct {
	*builtin.BuiltinTool
}

// NewRedactTextTool creates a redact_text tool.
func NewRedactTextTool(tenantID string) *RedactTextTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "redact_text",
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": "Redact Text", "zh_Hans": "Redact Text"},
			Icon:     "shield-alert",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Detect and redact sensitive information from text.", "zh_Hans": "Detect and redact sensitive information from text."},
			LLM:   "Detect and redact sensitive information from text. Supports phone, email, Chinese ID cards, bank cards, names, companies, customers, order IDs, contract IDs, secrets, tokens, passwords, IPs, and sensitive URL parameters. Never return complete original sensitive values in field lists.",
		},
		Parameters: []tools.ToolParameter{
			{Name: "text", Label: tools.I18nText{"en_US": "Text", "zh_Hans": "Text"}, HumanDescription: tools.I18nText{"en_US": "Text to redact.", "zh_Hans": "Text to redact."}, LLMDescription: "Required text to redact. Do not pass binary file contents.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, SupportVariable: true},
			{Name: "level", Label: tools.I18nText{"en_US": "Level", "zh_Hans": "Level"}, HumanDescription: tools.I18nText{"en_US": "Redaction level.", "zh_Hans": "Redaction level."}, LLMDescription: "Redaction level: low, medium, or high. Defaults to medium. Use high for external sharing, training data, logs, contracts, resumes, and customer data.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: defaultRedactionLevel, SupportVariable: true, Options: selectOptions("low", "medium", "high")},
			{Name: "strategy", Label: tools.I18nText{"en_US": "Strategy", "zh_Hans": "Strategy"}, HumanDescription: tools.I18nText{"en_US": "Redaction strategy.", "zh_Hans": "Redaction strategy."}, LLMDescription: "Redaction strategy: auto, partial, full, or label. Defaults to auto. High-risk secrets are always fully hidden unless label strategy is requested.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: defaultRedactionStrategy, SupportVariable: true, Options: selectOptions("auto", "partial", "full", "label")},
			{Name: "preserve_rules", Label: tools.I18nText{"en_US": "Preserve Rules", "zh_Hans": "Preserve Rules"}, HumanDescription: tools.I18nText{"en_US": "Optional JSON preserve rules.", "zh_Hans": "Optional JSON preserve rules."}, LLMDescription: "Optional object or JSON object string: keep_last_digits 0-8, keep_email_domain, keep_city, keep_url_domain.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
			{Name: "entity_types", Label: tools.I18nText{"en_US": "Entity Types", "zh_Hans": "Entity Types"}, HumanDescription: tools.I18nText{"en_US": "Optional entity type filter.", "zh_Hans": "Optional entity type filter."}, LLMDescription: "Optional array, JSON array string, or comma-separated entity type filter.", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: false, SupportVariable: true},
			{Name: "locale", Label: tools.I18nText{"en_US": "Locale", "zh_Hans": "Locale"}, HumanDescription: tools.I18nText{"en_US": "Locale hint.", "zh_Hans": "Locale hint."}, LLMDescription: "Locale hint: auto, zh-CN, or en-US. Defaults to auto.", Type: tools.ToolParameterTypeSelect, Form: tools.ToolParameterFormLLM, Required: false, Default: "auto", SupportVariable: true, Options: selectOptions("auto", "zh-CN", "en-US")},
			{Name: "include_field_list", Label: tools.I18nText{"en_US": "Include Field List", "zh_Hans": "Include Field List"}, HumanDescription: tools.I18nText{"en_US": "Whether to return redacted field summaries.", "zh_Hans": "Whether to return redacted field summaries."}, LLMDescription: "Whether to include a field list. The list contains only redacted replacements, never complete original values. Defaults to true.", Type: tools.ToolParameterTypeBoolean, Form: tools.ToolParameterFormLLM, Required: false, Default: true, SupportVariable: true},
		},
		OutputType: "json",
		Tags:       []string{"privacy", "security", "redaction"},
	}
	return &RedactTextTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *RedactTextTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &RedactTextTool{BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime)}
}

func (t *RedactTextTool) Invoke(ctx context.Context, userID string, toolParameters map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	text := rawString(toolParameters["text"])
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text is required")
	}
	if len([]byte(text)) > maxRedactionInputBytes {
		return nil, fmt.Errorf("text exceeds %d bytes", maxRedactionInputBytes)
	}
	options, err := parseOptions(toolParameters)
	if err != nil {
		return nil, err
	}
	result := redactText(text, options)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(result)}, nil
}

func redactText(input string, options redactionOptions) map[string]interface{} {
	stats := map[string]int{}
	risks := map[string]string{}
	examples := map[string]string{}
	redacted := redactURLs(input, options, stats, risks, examples)
	for _, rule := range redactionRules {
		if !options.wants(rule.EntityType) {
			continue
		}
		redacted = applyRule(redacted, rule, options, stats, risks, examples)
	}
	fields := redactedFields(stats, risks, examples, options.IncludeFieldList)
	warnings := []string{
		"Rule-based redaction may miss context-dependent names, organization aliases, screenshots, images, or malformed data. Review before external sharing.",
	}
	if len(stats) == 0 {
		warnings = append(warnings, "No supported sensitive fields were detected by the current rule set.")
	}
	return map[string]interface{}{
		"redacted_text": redacted,
		"level":         options.Level,
		"strategy":      options.Strategy,
		"locale":        options.Locale,
		"stats":         stats,
		"fields":        fields,
		"warnings":      warnings,
	}
}

func redactURLs(input string, options redactionOptions, stats map[string]int, risks map[string]string, examples map[string]string) string {
	if !options.wants("url_parameter") {
		return input
	}
	return urlPattern.ReplaceAllStringFunc(input, func(match string) string {
		replacement, count := redactURL(match, options)
		if count == 0 {
			return match
		}
		stats["url_parameter"] += count
		risks["url_parameter"] = "high"
		examples["url_parameter"] = fullReplacement("url_parameter")
		return replacement
	})
}

func applyRule(input string, rule redactionRule, options redactionOptions, stats map[string]int, risks map[string]string, examples map[string]string) string {
	matches := rule.Pattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input
	}
	type edit struct {
		start       int
		end         int
		replacement string
	}
	edits := []edit{}
	for _, match := range matches {
		start, end := match[0], match[1]
		if rule.ValueGroup > 0 && len(match) > rule.ValueGroup*2+1 && match[rule.ValueGroup*2] >= 0 {
			start = match[rule.ValueGroup*2]
			end = match[rule.ValueGroup*2+1]
		}
		value := input[start:end]
		if rule.EntityType == "bank_card" && !looksLikeBankCard(value) {
			continue
		}
		replacement := replacementFor(rule.EntityType, value, options)
		edits = append(edits, edit{start: start, end: end, replacement: replacement})
		stats[rule.EntityType]++
		risks[rule.EntityType] = rule.Risk
		if examples[rule.EntityType] == "" {
			examples[rule.EntityType] = replacement
		}
	}
	if len(edits) == 0 {
		return input
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := input
	for _, edit := range edits {
		out = out[:edit.start] + edit.replacement + out[edit.end:]
	}
	return out
}

func looksLikeBankCard(value string) bool {
	digits := digitsOnly(value)
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	if len(digits) == 18 && regexp.MustCompile(`^[1-9]\d{5}(?:18|19|20)`).MatchString(digits) {
		return false
	}
	return luhnValid(digits)
}

func luhnValid(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		n := int(digits[i] - '0')
		if double {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		double = !double
	}
	return sum%10 == 0
}

func redactedFields(stats map[string]int, risks map[string]string, examples map[string]string, include bool) []map[string]interface{} {
	if !include {
		return []map[string]interface{}{}
	}
	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, map[string]interface{}{
			"type":        key,
			"replacement": examples[key],
			"count":       stats[key],
			"risk":        risks[key],
		})
	}
	return fields
}

func selectOptions(values ...string) []tools.ToolParameterOption {
	options := make([]tools.ToolParameterOption, 0, len(values))
	for _, value := range values {
		options = append(options, tools.ToolParameterOption{Value: value, Label: tools.I18nText{"en_US": value, "zh_Hans": value}})
	}
	return options
}
