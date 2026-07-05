package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func turnStateContinuationSummary(message *runtimemodel.Message) map[string]interface{} {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	state := mapFromOperationContext(metadataValue(message.Metadata, "turn_state"))
	items := mapSliceFromAny(state["items"])
	if len(items) == 0 {
		return nil
	}
	outItems := make([]map[string]interface{}, 0, 12)
	for _, item := range items {
		kind := strings.TrimSpace(stringFromAny(item["kind"]))
		visibility := strings.TrimSpace(stringFromAny(item["visibility"]))
		if visibility == "user_visible" && kind == "user_deliverable" {
			continue
		}
		if kind == "" {
			continue
		}
		compact := map[string]interface{}{
			"kind":       kind,
			"visibility": firstNonEmptyString(visibility, "model_only"),
		}
		for _, key := range []string{"key", "value", "title", "source"} {
			if value := strings.TrimSpace(stringFromAny(item[key])); value != "" {
				limit := 500
				if key == "key" {
					limit = 120
				}
				compact[key] = truncateRunes(value, limit)
			}
		}
		if usedFor := mapSliceOrStringListForPrompt(item["used_for"], 8, 120); len(usedFor) > 0 {
			compact["used_for"] = usedFor
		}
		if confidence, ok := floatValue(item["confidence"]); ok {
			compact["confidence"] = confidence
		}
		outItems = append(outItems, compact)
		if len(outItems) >= 12 {
			break
		}
	}
	if len(outItems) == 0 {
		return nil
	}
	return map[string]interface{}{
		"items": mapsToInterfaceSlice(outItems),
		"instructions": []string{
			"Treat these turn_state items as authoritative state recorded earlier in this same AIChat turn.",
			"Reuse exact working_fact values for later tool arguments and final answers instead of re-deriving placeholders.",
			"If a turn_state item came from an earlier tool result and satisfies a later dependency, do not rerun the same earlier tool or navigate back to the same earlier page merely to rederive that fact.",
			"If later tool/page evidence contradicts a turn_state item, update the state with submit_turn_state before proceeding.",
		},
	}
}

func mapSliceOrStringListForPrompt(value interface{}, maxItems int, maxRunes int) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringFromAny(item))
			if text == "" {
				continue
			}
			out = append(out, truncateRunes(text, maxRunes))
			if len(out) >= maxItems {
				break
			}
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(item)
			if text == "" {
				continue
			}
			out = append(out, truncateRunes(text, maxRunes))
			if len(out) >= maxItems {
				break
			}
		}
		return out
	default:
		return nil
	}
}
