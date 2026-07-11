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

	turnStateSurfaceContextualSidebar = "contextual_sidebar"
	turnStateCheckpointMaxRunes       = 220
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
	for _, item := range items {
		trace, ok := contextualTurnStateCheckpoint(prepared, item)
		if !ok {
			continue
		}
		r.emitIntermediateAnswer(ctx, prepared, callID, trace, onEvent)
	}

	trace := skills.SkillTrace{
		Kind:   "turn_state",
		Status: "success",
		Arguments: map[string]interface{}{
			"item_count": len(items),
		},
		Result: map[string]interface{}{
			"items": items,
		},
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status": "recorded",
		"items":  items,
		"instruction": strings.Join([]string{
			"Turn state has been recorded for this same AIChat turn.",
			"Use working_fact, decision, assumption, and verification items as authoritative context for later steps after approvals, navigation, or client actions.",
			"If a later step needs a value that was not recorded and is not visible in current evidence, re-read or re-observe instead of guessing.",
			"Only user_deliverable items are directly visible to the user; the contextual sidebar may show a brief checkpoint for file-derived working facts so the user can verify the source conclusion.",
		}, " "),
	}), false, false)
}

func contextualTurnStateCheckpoint(prepared *PreparedChat, item map[string]interface{}) (skills.SkillTrace, bool) {
	if prepared == nil || strings.TrimSpace(prepared.Surface) != turnStateSurfaceContextualSidebar {
		return skills.SkillTrace{}, false
	}
	if strings.TrimSpace(stringFromInterface(item["visibility"])) != turnStateVisibilityModelOnly {
		return skills.SkillTrace{}, false
	}
	kind := strings.TrimSpace(stringFromInterface(item["kind"]))
	if kind != turnStateKindWorkingFact && kind != turnStateKindDecision && kind != turnStateKindVerification {
		return skills.SkillTrace{}, false
	}
	value := strings.TrimSpace(stringFromInterface(item["value"]))
	if value == "" {
		value = strings.TrimSpace(stringFromInterface(item["content"]))
	}
	if value == "" || !turnStateCheckpointIsUsefulForUser(item) {
		return skills.SkillTrace{}, false
	}
	title, message := turnStateCheckpointText(prepared, item, value)
	return skills.SkillTrace{
		Kind:    "intermediate_answer",
		Title:   title,
		Message: message,
		Status:  "success",
		Arguments: map[string]interface{}{
			"title":           title,
			"turn_state_kind": kind,
			"turn_state_key":  strings.TrimSpace(stringFromInterface(item["key"])),
			"source":          strings.TrimSpace(stringFromInterface(item["source"])),
		},
	}, true
}

func turnStateCheckpointIsUsefulForUser(item map[string]interface{}) bool {
	key := strings.ToLower(strings.TrimSpace(stringFromInterface(item["key"])))
	source := strings.ToLower(strings.TrimSpace(stringFromInterface(item["source"])))
	if strings.HasPrefix(source, "file-reader/") || strings.Contains(source, "read_file") {
		return true
	}
	return strings.Contains(key, "summary") ||
		strings.Contains(key, "theme") ||
		strings.Contains(key, "content") ||
		strings.Contains(key, "topic")
}

func turnStateCheckpointText(prepared *PreparedChat, item map[string]interface{}, value string) (string, string) {
	preview := trimRunes(value, turnStateCheckpointMaxRunes)
	source := strings.ToLower(strings.TrimSpace(stringFromInterface(item["source"])))
	fileDerived := strings.HasPrefix(source, "file-reader/") || strings.Contains(source, "read_file")
	if turnStatePrefersChinese(prepared) {
		title := "\u5df2\u8bb0\u5f55\u7ed3\u8bba"
		prefix := "\u5df2\u8bb0\u5f55\u5173\u952e\u7ed3\u8bba"
		if fileDerived {
			prefix = "\u5df2\u8bb0\u5f55\u6587\u4ef6\u5c0f\u7ed3"
		}
		return title, fmt.Sprintf("%s: %s", prefix, preview)
	}
	title := "Saved note"
	prefix := "Saved key note"
	if fileDerived {
		prefix = "Saved file summary"
	}
	return title, fmt.Sprintf("%s: %s", prefix, preview)
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

func normalizeTurnStateItem(raw map[string]interface{}) map[string]interface{} {
	kind := normalizeTurnStateKind(stringFromInterface(raw["kind"]))
	if kind == "" {
		return nil
	}
	visibility := normalizeTurnStateVisibility(stringFromInterface(raw["visibility"]), kind)
	value := turnStateStringValue(raw["value"])
	content := trimRunes(stringFromInterface(raw["content"]), 16000)
	if kind == turnStateKindUserDeliverable && content == "" {
		content = trimRunes(value, 16000)
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
		item["value"] = trimRunes(value, 4000)
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
