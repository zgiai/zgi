package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	turnStateKindWorkingFact     = "working_fact"
	turnStateKindUserDeliverable = "user_deliverable"
	turnStateKindDecision        = "decision"
	turnStateKindAssumption      = "assumption"
	turnStateKindVerification    = "verification"

	turnStateVisibilityModelOnly   = "model_only"
	turnStateVisibilityUserVisible = "user_visible"
	turnStateVisibilityAudit       = "audit"

	turnStateTextValueMaxRunes = 1024
)

func (r *Runner) handleTurnStateCall(
	ctx context.Context,
	prepared *PreparedChat,
	callID string,
	args map[string]interface{},
	onEvent func(Event) error,
) skillStepResult {
	items, err := normalizeTurnStateItems(args)
	if err != nil {
		trace := failedSkillTrace("turn_state", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call submit_turn_state again with one to eight valid items; use working_fact/model_only for internal facts or user_deliverable/user_visible for content that should be shown")), false, false)
	}

	for _, item := range items {
		if strings.TrimSpace(stringFromInterface(item["kind"])) != turnStateKindUserDeliverable ||
			strings.TrimSpace(stringFromInterface(item["visibility"])) != turnStateVisibilityUserVisible {
			continue
		}
		content := strings.TrimSpace(stringFromInterface(item["content"]))
		if content == "" {
			content = strings.TrimSpace(stringFromInterface(item["value"]))
		}
		if content == "" {
			continue
		}
		trace := skills.SkillTrace{
			Kind:    "intermediate_answer",
			Title:   strings.TrimSpace(stringFromInterface(item["title"])),
			Message: content,
			Status:  "success",
			Arguments: map[string]interface{}{
				"title":           strings.TrimSpace(stringFromInterface(item["title"])),
				"turn_state_kind": turnStateKindUserDeliverable,
			},
		}
		r.emitIntermediateAnswer(ctx, prepared, callID, trace, onEvent)
	}
	trace := skills.SkillTrace{
		Kind:   "turn_state",
		Status: "success",
		Arguments: map[string]interface{}{
			"item_count": len(items),
			"call_id":    strings.TrimSpace(callID),
		},
		Result: map[string]interface{}{
			"items": items,
		},
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status":   "recorded",
		"receipts": turnStateItemReceipts(items),
		"instruction": strings.Join([]string{
			"Turn state has been recorded for this same AIChat turn.",
			"Use working_fact, decision, assumption, and verification items as authoritative context for later steps after approvals, navigation, or client actions.",
			"If a later step needs a value that was not recorded and is not visible in current evidence, re-read or re-observe instead of guessing.",
			"The tool result contains compact receipts rather than echoing recorded values; the authoritative values are restored by the runtime when needed.",
			"Only user_deliverable items with user_visible visibility are directly visible to the user. Model-only working facts are never rendered as chat content.",
		}, " "),
	}), false, false)
}

func turnStateItemReceipts(items []map[string]interface{}) []map[string]interface{} {
	receipts := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		receipt := map[string]interface{}{
			"kind":       stringFromInterface(item["kind"]),
			"visibility": stringFromInterface(item["visibility"]),
		}
		for _, key := range []string{"key", "source", "title"} {
			if value := strings.TrimSpace(stringFromInterface(item[key])); value != "" {
				receipt[key] = value
			}
		}
		value := strings.TrimSpace(firstNonEmptyString(item["value"], item["content"]))
		if value != "" {
			receipt["recorded_chars"] = len([]rune(value))
		}
		receipts = append(receipts, receipt)
	}
	return receipts
}

func turnStatePrefersChinese(prepared *PreparedChat) bool {
	if prepared == nil {
		return false
	}
	for _, r := range prepared.Query {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func normalizeTurnStateItems(args map[string]interface{}) ([]map[string]interface{}, error) {
	items := mapSliceFromAny(args["items"])
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: submit_turn_state items are required", ErrInvalidInput)
	}
	if len(items) > 8 {
		items = items[:8]
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, raw := range items {
		if err := validateTurnStateTextBounds(raw); err != nil {
			return nil, err
		}
		item := normalizeTurnStateItem(raw)
		if len(item) == 0 {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: submit_turn_state has no valid items", ErrInvalidInput)
	}
	return out, nil
}

func validateTurnStateTextBounds(raw map[string]interface{}) error {
	for _, field := range []string{"value", "content"} {
		text, ok := raw[field].(string)
		if !ok || len([]rune(text)) <= turnStateTextValueMaxRunes {
			continue
		}
		return fmt.Errorf(
			"%w: submit_turn_state %s exceeds %d Unicode characters; store the full content as an artifact and record only {ref, digest, summary}",
			ErrInvalidInput,
			field,
			turnStateTextValueMaxRunes,
		)
	}
	return nil
}

func normalizeTurnStateItem(raw map[string]interface{}) map[string]interface{} {
	kind := normalizeTurnStateKind(stringFromInterface(raw["kind"]))
	if kind == "" {
		return nil
	}
	visibility := normalizeTurnStateVisibility(stringFromInterface(raw["visibility"]), kind)
	value := turnStateStringValue(raw["value"])
	content := trimRunes(stringFromInterface(raw["content"]), turnStateTextValueMaxRunes)
	if kind == turnStateKindUserDeliverable && content == "" {
		content = trimRunes(value, turnStateTextValueMaxRunes)
	}
	if kind != turnStateKindUserDeliverable && value == "" {
		value = content
	}
	if strings.TrimSpace(value) == "" && strings.TrimSpace(content) == "" {
		return nil
	}
	item := map[string]interface{}{
		"kind":       kind,
		"visibility": visibility,
	}
	if key := normalizeTurnStateKey(stringFromInterface(raw["key"])); key != "" {
		item["key"] = key
	}
	if value != "" {
		item["value"] = trimRunes(value, turnStateTextValueMaxRunes)
	}
	if title := trimRunes(stringFromInterface(raw["title"]), 120); title != "" {
		item["title"] = title
	}
	if content != "" {
		item["content"] = content
	}
	if source := trimRunes(stringFromInterface(raw["source"]), 200); source != "" {
		item["source"] = source
	}
	if usedFor := normalizeTurnStateStringSlice(raw["used_for"], 8, 120); len(usedFor) > 0 {
		item["used_for"] = usedFor
	}
	if confidence, ok := normalizeTurnStateConfidence(raw["confidence"]); ok {
		item["confidence"] = confidence
	}
	return item
}

func normalizeTurnStateKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case turnStateKindWorkingFact, "fact", "working-fact":
		return turnStateKindWorkingFact
	case turnStateKindUserDeliverable, "deliverable", "answer", "intermediate_answer":
		return turnStateKindUserDeliverable
	case turnStateKindDecision:
		return turnStateKindDecision
	case turnStateKindAssumption:
		return turnStateKindAssumption
	case turnStateKindVerification, "verify", "verification_result":
		return turnStateKindVerification
	default:
		return ""
	}
}

func normalizeTurnStateVisibility(value string, kind string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case turnStateVisibilityUserVisible, "visible", "user":
		return turnStateVisibilityUserVisible
	case turnStateVisibilityAudit:
		return turnStateVisibilityAudit
	case turnStateVisibilityModelOnly, "internal", "":
		if kind == turnStateKindUserDeliverable {
			return turnStateVisibilityUserVisible
		}
		return turnStateVisibilityModelOnly
	default:
		if kind == turnStateKindUserDeliverable {
			return turnStateVisibilityUserVisible
		}
		return turnStateVisibilityModelOnly
	}
}

func normalizeTurnStateKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return trimRunes(value, 120)
}

func turnStateStringValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", typed))
		}
		return strings.TrimSpace(string(data))
	}
}

func normalizeTurnStateStringSlice(value interface{}, maxItems int, maxRunes int) []interface{} {
	var raw []interface{}
	switch typed := value.(type) {
	case []interface{}:
		raw = typed
	case []string:
		raw = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return nil
	}
	out := make([]interface{}, 0, len(raw))
	for _, item := range raw {
		text := trimRunes(turnStateStringValue(item), maxRunes)
		if text == "" {
			continue
		}
		out = append(out, text)
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func normalizeTurnStateConfidence(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return clampTurnStateConfidence(typed), true
	case float32:
		return clampTurnStateConfidence(float64(typed)), true
	case int:
		return clampTurnStateConfidence(float64(typed)), true
	case int64:
		return clampTurnStateConfidence(float64(typed)), true
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return clampTurnStateConfidence(parsed), true
	default:
		return 0, false
	}
}

func clampTurnStateConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
