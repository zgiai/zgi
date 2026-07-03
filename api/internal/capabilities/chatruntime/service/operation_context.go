package service

import "strings"

const (
	operationLedgerVersion        = "operation_ledger.v1"
	operationLedgerStatusObserved = "observed"

	maxOperationLedgerResources      = 8
	maxOperationLedgerCapabilities   = 8
	maxOperationLedgerFieldRunes     = 120
	maxOperationLedgerRiskItems      = 6
	maxOperationLedgerRiskFieldRunes = 64
)

type operationSummaryKey struct {
	Source string
	Target string
}

var operationResourceSummaryKeys = []operationSummaryKey{
	{Source: "id", Target: "id"},
	{Source: "resource_id", Target: "id"},
	{Source: "resource_id", Target: "resource_id"},
	{Source: "agent_id", Target: "agent_id"},
	{Source: "type", Target: "type"},
	{Source: "resource_type", Target: "type"},
	{Source: "resource_type", Target: "resource_type"},
	{Source: "kind", Target: "kind"},
	{Source: "name", Target: "name"},
	{Source: "title", Target: "name"},
	{Source: "title", Target: "title"},
	{Source: "label", Target: "label"},
	{Source: "href", Target: "href"},
	{Source: "source", Target: "source"},
	{Source: "status", Target: "status"},
	{Source: "scope", Target: "scope"},
	{Source: "workspace_id", Target: "workspace_id"},
}

var operationCapabilitySummaryKeys = []operationSummaryKey{
	{Source: "id", Target: "id"},
	{Source: "capability_id", Target: "id"},
	{Source: "tool_id", Target: "id"},
	{Source: "type", Target: "type"},
	{Source: "capability_type", Target: "type"},
	{Source: "kind", Target: "kind"},
	{Source: "name", Target: "name"},
	{Source: "tool_name", Target: "name"},
	{Source: "label", Target: "label"},
	{Source: "action", Target: "action"},
	{Source: "source", Target: "source"},
	{Source: "status", Target: "status"},
	{Source: "scope", Target: "scope"},
	{Source: "requires_approval", Target: "requires_approval"},
	{Source: "requires_confirmation", Target: "requires_confirmation"},
}

func normalizeOperationContext(input map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	if input == nil {
		return nil, nil
	}
	resources, resourceCount := summarizeOperationItems(input, []string{"resources", "resource"}, maxOperationLedgerResources, operationResourceSummaryKeys)
	capabilities, capabilityCount := summarizeOperationItems(input, []string{"capabilities", "capability"}, maxOperationLedgerCapabilities, operationCapabilitySummaryKeys)

	summary := map[string]interface{}{
		"resource_count":   resourceCount,
		"capability_count": capabilityCount,
		"risk_summary":     summarizeOperationRisk(input),
	}
	if len(resources) > 0 {
		summary["resources"] = resources
	}
	if len(capabilities) > 0 {
		summary["capabilities"] = capabilities
	}
	if resourceCount > len(resources) {
		summary["resources_truncated"] = true
	}
	if capabilityCount > len(capabilities) {
		summary["capabilities_truncated"] = true
	}

	ledger := copyStringAnyMap(summary)
	ledger["version"] = operationLedgerVersion
	ledger["status"] = operationLedgerStatusObserved
	return summary, ledger
}

func summarizeOperationItems(input map[string]interface{}, keys []string, limit int, summaryKeys []operationSummaryKey) ([]map[string]interface{}, int) {
	items := operationItemsFromKeys(input, keys)
	if len(items) == 0 || limit <= 0 {
		return nil, len(items)
	}
	out := make([]map[string]interface{}, 0, minInt(len(items), limit))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		summary, ok := summarizeOperationItem(item, summaryKeys)
		if !ok {
			continue
		}
		out = append(out, summary)
	}
	return out, len(items)
}

func operationItemsFromKeys(input map[string]interface{}, keys []string) []interface{} {
	var items []interface{}
	for _, key := range keys {
		items = append(items, operationItemsFromValue(input[key])...)
	}
	return items
}

func operationItemsFromValue(value interface{}) []interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case []interface{}:
		return typed
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	case []string:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	case map[string]interface{}:
		return []interface{}{typed}
	case string:
		return []interface{}{typed}
	default:
		return nil
	}
}

func summarizeOperationItem(item interface{}, keys []operationSummaryKey) (map[string]interface{}, bool) {
	switch typed := item.(type) {
	case string:
		text, ok := sanitizedOperationString(typed, maxOperationLedgerFieldRunes)
		if !ok {
			return nil, false
		}
		return map[string]interface{}{"id": text}, true
	case map[string]interface{}:
		out := make(map[string]interface{}, len(keys))
		for _, key := range keys {
			if _, exists := out[key.Target]; exists {
				continue
			}
			value, ok := sanitizedOperationScalar(typed[key.Source])
			if !ok {
				continue
			}
			out[key.Target] = value
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func sanitizedOperationScalar(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		return sanitizedOperationString(typed, maxOperationLedgerFieldRunes)
	case bool:
		return typed, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	default:
		return nil, false
	}
}

func sanitizedOperationString(value string, limit int) (string, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", false
	}
	return truncateRunes(text, limit), true
}

func summarizeOperationRisk(input map[string]interface{}) map[string]interface{} {
	riskSummary := mapFromOperationContext(input["risk_summary"])
	risk := mapFromOperationContext(input["risk"])
	contextSummary := mapFromOperationContext(input["summary"])
	summary := map[string]interface{}{
		"level": "unspecified",
	}
	if level := firstOperationRiskToken(
		input["risk_level"],
		input["highest_risk"],
		riskSummary["level"],
		riskSummary["severity"],
		risk["level"],
		risk["severity"],
		contextSummary["level"],
		contextSummary["highest_risk"],
		input["risk_summary"],
	); level != "" {
		summary["level"] = strings.ToLower(level)
	}
	if approval, ok := firstOperationBool(
		input["requires_approval"],
		input["approval_required"],
		input["requires_confirmation"],
		riskSummary["requires_approval"],
		riskSummary["approval_required"],
		riskSummary["requires_confirmation"],
		risk["requires_approval"],
		risk["approval_required"],
		risk["requires_confirmation"],
		contextSummary["requires_approval"],
		contextSummary["approval_required"],
		contextSummary["requires_confirmation"],
	); ok {
		summary["requires_approval"] = approval
	}
	if categories := firstOperationTokenList(
		input["risk_categories"],
		riskSummary["categories"],
		riskSummary["signals"],
		risk["categories"],
		risk["signals"],
	); len(categories) > 0 {
		summary["categories"] = categories
	}
	return summary
}

func mapFromOperationContext(value interface{}) map[string]interface{} {
	typed, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return typed
}

func firstOperationRiskToken(values ...interface{}) string {
	for _, value := range values {
		token, ok := operationRiskToken(value)
		if ok {
			return token
		}
	}
	return ""
}

func operationRiskToken(value interface{}) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	token := strings.TrimSpace(text)
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", false
	}
	if len([]rune(token)) > maxOperationLedgerRiskFieldRunes {
		return "", false
	}
	return token, true
}

func firstOperationBool(values ...interface{}) (bool, bool) {
	for _, value := range values {
		typed, ok := value.(bool)
		if ok {
			return typed, true
		}
	}
	return false, false
}

func firstOperationTokenList(values ...interface{}) []string {
	for _, value := range values {
		items := operationTokenList(value)
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func operationTokenList(value interface{}) []string {
	var raw []interface{}
	switch typed := value.(type) {
	case []interface{}:
		raw = typed
	case []string:
		raw = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	case string:
		raw = []interface{}{typed}
	default:
		return nil
	}
	out := make([]string, 0, minInt(len(raw), maxOperationLedgerRiskItems))
	seen := map[string]struct{}{}
	for _, item := range raw {
		if len(out) >= maxOperationLedgerRiskItems {
			break
		}
		token, ok := operationRiskToken(item)
		if !ok {
			continue
		}
		normalized := strings.ToLower(token)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
