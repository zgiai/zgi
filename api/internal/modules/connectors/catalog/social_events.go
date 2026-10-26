package catalog

// socialEventOperations expose the owner-scoped local queue through the same
// native authorization and confirmation boundary as external connector actions.
func socialEventOperations(provider string) []Operation {
	if provider != "facebook-social" {
		return nil
	}
	var operations []Operation
	for _, entry := range []struct {
		id, en, zh string
		effect     OperationEffect
	}{
		{"event.list", "Read queued Page events", "读取主页事件队列", OperationEffectRead},
		{"event.ack", "Acknowledge queued Page events", "确认已处理主页事件", OperationEffectUpdate},
	} {
		properties := map[string]any{}
		required := []any{}
		if entry.id == "event.list" {
			properties["after_id"] = map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991), "default": 0}
			properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 20}
		} else {
			properties["through_id"] = map[string]any{"type": "integer", "minimum": 1, "maximum": int64(9007199254740991)}
			required = append(required, "through_id")
		}
		operations = append(operations, Operation{ID: provider + "." + entry.id, ToolName: "facebook_social_" + map[string]string{"event.list": "event_list", "event.ack": "event_ack"}[entry.id], SkillID: "connector-runtime", Name: entry.en, NameI18n: LocalizedText{"en-US": entry.en, "zh-Hans": entry.zh}, Description: entry.en + " for the authorized local Page connection.", DescriptionI18n: LocalizedText{"en-US": entry.en + " for the authorized local Page connection.", "zh-Hans": entry.zh + "，仅限已授权的当前主页连接。"}, Effect: entry.effect, RiskHint: OperationRiskHintLow, DataEgress: false, Idempotent: true,
			InputSchema:  map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false, "properties": properties, "required": required},
			OutputSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []any{"provider", "transport", "data"}, "properties": map[string]any{"provider": map[string]any{"const": provider}, "transport": map[string]any{"const": "local"}, "data": map[string]any{"type": "object"}}},
		})
	}
	return operations
}
