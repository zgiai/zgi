package tools

import "testing"

func TestJSONSchemaToToolParameters(t *testing.T) {
	schema := map[string]interface{}{
		"type":                 "object",
		"x-zgi-property-order": []string{"query", "mode", "targets", "published_on", "published_after", "limit"},
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":  "string",
				"title": "Query",
				"title_i18n": map[string]string{
					"en-US": "Query", "zh-Hans": "查询内容",
				},
				"description": "Search query",
				"description_i18n": map[string]string{
					"en-US": "Search query", "zh-Hans": "输入要搜索的公开信息",
				},
				"placeholder_i18n": map[string]string{
					"en-US": "Enter a query", "zh-Hans": "输入搜索内容",
				},
			},
			"published_after": map[string]interface{}{
				"type": "string", "format": "date-time",
				"title_i18n": map[string]string{"en-US": "Published after", "zh-Hans": "最早发布日期"},
			},
			"published_on": map[string]interface{}{
				"type": "string", "format": "date",
				"title_i18n": map[string]string{"en-US": "Published on", "zh-Hans": "发布日期"},
			},
			"limit": map[string]interface{}{
				"type":    "integer",
				"minimum": float64(1),
				"maximum": float64(10),
			},
			"mode": map[string]interface{}{
				"type": "string",
				"enum": []string{"fast", "auto"},
				"enum_labels_i18n": map[string]map[string]string{
					"en-US":   {"fast": "Fast", "auto": "Automatic"},
					"zh-Hans": {"fast": "快速", "auto": "自动"},
				},
			},
			"targets": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"server_owned": map[string]interface{}{
				"type":     "string",
				"readOnly": true,
			},
		},
		"required": []interface{}{"query"},
	}

	parameters, err := JSONSchemaToToolParameters(schema)
	if err != nil {
		t.Fatalf("JSONSchemaToToolParameters() error = %v", err)
	}
	if len(parameters) != 6 {
		t.Fatalf("parameter count = %d, want 6", len(parameters))
	}
	wantOrder := []string{"query", "mode", "targets", "published_on", "published_after", "limit"}
	for index, name := range wantOrder {
		if parameters[index].Name != name {
			t.Fatalf("parameter order[%d] = %q, want %q", index, parameters[index].Name, name)
		}
	}
	byName := make(map[string]ToolParameter, len(parameters))
	for _, parameter := range parameters {
		byName[parameter.Name] = parameter
	}
	if parameter := byName["query"]; parameter.Type != ToolParameterTypeString || !parameter.Required || !parameter.SupportVariable {
		t.Fatalf("query parameter = %#v", parameter)
	} else if parameter.Label["zh_Hans"] != "查询内容" {
		t.Fatalf("query label = %#v", parameter.Label)
	} else if parameter.HumanDescription["zh_Hans"] != "输入要搜索的公开信息" || parameter.Placeholder["zh_Hans"] != "输入搜索内容" {
		t.Fatalf("query localization = description %#v placeholder %#v", parameter.HumanDescription, parameter.Placeholder)
	}
	if parameter := byName["limit"]; parameter.Type != ToolParameterTypeNumber || parameter.MinValue == nil || *parameter.MinValue != 1 || parameter.MaxValue == nil || *parameter.MaxValue != 10 {
		t.Fatalf("limit parameter = %#v", parameter)
	}
	if parameter := byName["mode"]; parameter.Type != ToolParameterTypeSelect || len(parameter.Options) != 2 {
		t.Fatalf("mode parameter = %#v", parameter)
	} else if parameter.Options[0].Label["zh_Hans"] != "快速" {
		t.Fatalf("mode options = %#v", parameter.Options)
	}
	if parameter := byName["targets"]; parameter.Type != ToolParameterTypeStringList {
		t.Fatalf("targets parameter = %#v", parameter)
	}
	if parameter := byName["published_after"]; parameter.Type != ToolParameterTypeDateTime {
		t.Fatalf("published_after parameter = %#v", parameter)
	}
	if parameter := byName["published_on"]; parameter.Type != ToolParameterTypeDate {
		t.Fatalf("published_on parameter = %#v", parameter)
	}
	if _, exists := byName["server_owned"]; exists {
		t.Fatal("read-only property must not be exposed")
	}
}

func TestJSONSchemaToToolParametersDoesNotLeakEnglishDescriptionIntoChineseUI(t *testing.T) {
	parameters, err := JSONSchemaToToolParameters(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"owner": map[string]interface{}{
				"type":        "string",
				"description": "Repository owner.",
				"title_i18n":  map[string]string{"en-US": "Repository owner", "zh-Hans": "仓库所有者"},
			},
		},
	})
	if err != nil {
		t.Fatalf("JSONSchemaToToolParameters() error = %v", err)
	}
	if got := parameters[0].HumanDescription["zh_Hans"]; got != "仓库所有者" {
		t.Fatalf("Chinese description = %q, want localized title fallback", got)
	}
}

func TestJSONSchemaToToolParametersRejectsNonObjectProperties(t *testing.T) {
	_, err := JSONSchemaToToolParameters(map[string]interface{}{"type": "string"})
	if err == nil {
		t.Fatal("expected object-property error")
	}
}
