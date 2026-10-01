package catalog

import "encoding/json"

func IsWeComResourceRead(id string) bool {
	switch id {
	case "wecom.documents.search", "wecom.documents.get", "wecom.calendar.list", "wecom.calendar.get", "wecom.calendar.search", "wecom.todos.list", "wecom.todos.get":
		return true
	}
	return false
}

// WeComReadDefinition adds resource reads in a new package version. The bot
// authentication contract and the original three operation contracts stay exact.
func WeComReadDefinition() Definition {
	d := messagingDefinition("wecom")
	d.PluginVersion = "1.1.0"
	textList := func(max int) map[string]any {
		return map[string]any{"type": "array", "items": textSchema(200), "minItems": 1, "maxItems": max, "uniqueItems": true}
	}
	refs := map[string]any{"type": "array", "items": textSchema(8192), "minItems": 1, "maxItems": 20, "uniqueItems": true}
	page := func(max int) map[string]any {
		return map[string]any{"type": "integer", "minimum": 1, "maximum": max, "default": 10}
	}
	date := map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`}
	for _, item := range []struct {
		id, en, zh string
		input      map[string]any
	}{
		{"documents.search", "Search accessible documents", "搜索可访问文档", objectSchema(map[string]any{"keywords": textList(20), "cursor": textSchema(8192), "limit": page(50)}, "keywords")},
		{"documents.get", "Read a document", "读取文档", objectSchema(map[string]any{"resource_handle": textSchema(8192), "content_type": map[string]any{"type": "string", "enum": []string{"text", "markdown"}, "default": "markdown"}}, "resource_handle")},
		{"calendar.list", "List schedules", "列出日程", objectSchema(map[string]any{"begin_time": date, "end_time": date})},
		{"calendar.get", "Read schedules", "读取日程", objectSchema(map[string]any{"resource_handles": refs}, "resource_handles")},
		{"calendar.search", "Search schedules", "搜索日程", objectSchema(map[string]any{"keywords": textList(20), "organizer": textSchema(256), "has_attendees": map[string]any{"type": "array", "minItems": 1, "maxItems": 20, "items": objectSchema(map[string]any{"userid": textSchema(256)}, "userid")}, "begin_time": date, "end_time": date, "cursor": textSchema(8192), "limit": page(50)})},
		{"todos.list", "List accessible to-dos", "列出可访问待办", objectSchema(map[string]any{"keywords": textList(100), "status_filter": map[string]any{"type": "array", "maxItems": 2, "uniqueItems": true, "items": map[string]any{"enum": []string{"finished", "proceed"}}}, "create_begin_time": date, "create_end_time": date, "deadline_begin_time": date, "deadline_end_time": date, "cursor": textSchema(8192), "limit": page(20)})},
		{"todos.get", "Read to-dos", "读取待办", objectSchema(map[string]any{"resource_handles": refs}, "resource_handles")},
	} {
		op := messagingOperation("wecom", item.id, item.en, item.zh, "bot", false, item.input)
		op.OutputSchema = wecomReadOutput(op.ID)
		d.Operations = append(d.Operations, op)
	}
	encoded, _ := json.Marshal(d)
	_ = json.Unmarshal(encoded, &d)
	return d
}

func wecomReadOutput(id string) map[string]any {
	optionalText := func(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }
	row := objectSchema(map[string]any{
		"resource_handle": textSchema(8192), "title": optionalText(512), "description": optionalText(4000),
		"type": optionalText(64), "status": optionalText(64), "begin_time": optionalText(64), "end_time": optionalText(64),
		"timezone": objectSchema(map[string]any{"timezone_id": optionalText(128), "timezone_offset": map[string]any{"type": "integer"}}),
		"deadline": objectSchema(map[string]any{"type": map[string]any{"enum": []string{"date", "datetime"}}, "value": optionalText(64)}, "type", "value"),
		"success":  map[string]any{"type": "boolean"},
	}, "resource_handle")
	data := objectSchema(map[string]any{"items": map[string]any{"type": "array", "maxItems": 200, "items": row}, "has_more": map[string]any{"type": "boolean"}, "next_cursor": textSchema(8192), "not_returned": map[string]any{"type": "array", "maxItems": 20, "items": textSchema(8192)}}, "items")
	if id == "wecom.documents.get" {
		data = objectSchema(map[string]any{"title": optionalText(512), "content": optionalText(262144), "content_type": map[string]any{"enum": []string{"text", "markdown"}}, "truncated": map[string]any{"type": "boolean"}}, "content", "content_type", "truncated")
	}
	return objectSchema(map[string]any{"provider": map[string]any{"const": "wecom"}, "operation": map[string]any{"const": id}, "data": data}, "provider", "operation", "data")
}
