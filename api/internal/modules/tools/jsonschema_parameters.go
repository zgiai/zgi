package tools

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// JSONSchemaToToolParameters projects the top-level properties of a dynamic
// tool schema onto the parameter contract consumed by the existing Workflow
// Tool node. Runtime validation continues to use the original JSON Schema.
func JSONSchemaToToolParameters(schema map[string]interface{}) ([]ToolParameter, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	visible := ModelVisibleJSONSchema(schema)
	if len(visible) == 0 {
		return nil, nil
	}
	properties, ok := visible["properties"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tool input schema must define object properties")
	}
	required := jsonSchemaStringSet(visible["required"])
	names := orderedJSONSchemaPropertyNames(visible, properties)

	parameters := make([]ToolParameter, 0, len(names))
	for _, name := range names {
		definition, ok := properties[name].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tool input schema property %q must be an object", name)
		}
		parameter, err := jsonSchemaPropertyToToolParameter(name, definition, required[name])
		if err != nil {
			return nil, err
		}
		parameters = append(parameters, parameter)
	}
	return parameters, nil
}

// orderedJSONSchemaPropertyNames keeps workflow forms deterministic while
// allowing providers to declare a human-oriented field order. JSON object
// property order is not part of the schema contract, so map iteration cannot
// be used for UI layout. Unlisted properties remain available and are appended
// alphabetically for backward compatibility.
func orderedJSONSchemaPropertyNames(schema, properties map[string]interface{}) []string {
	names := make([]string, 0, len(properties))
	seen := make(map[string]struct{}, len(properties))
	for _, name := range jsonSchemaStringSlice(schema["x-zgi-property-order"]) {
		if _, exists := properties[name]; !exists {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		names = append(names, name)
		seen[name] = struct{}{}
	}
	remainder := make([]string, 0, len(properties)-len(names))
	for name := range properties {
		if _, exists := seen[name]; !exists {
			remainder = append(remainder, name)
		}
	}
	sort.Strings(remainder)
	return append(names, remainder...)
}

func jsonSchemaPropertyToToolParameter(name string, definition map[string]interface{}, required bool) (ToolParameter, error) {
	title := strings.TrimSpace(jsonSchemaString(definition["title"]))
	if title == "" {
		title = name
	}
	description := strings.TrimSpace(jsonSchemaString(definition["description"]))
	label := jsonSchemaI18nText(definition["title_i18n"], title)
	humanDescription := jsonSchemaI18nText(definition["description_i18n"], "")
	if strings.TrimSpace(humanDescription["en_US"]) == "" {
		humanDescription["en_US"] = firstNonBlank(description, label["en_US"], title)
	}
	// Never copy an English-only schema description into the Chinese UI. If a
	// provider has not supplied a localized description yet, the localized
	// field title is a safe and understandable fallback.
	if strings.TrimSpace(humanDescription["zh_Hans"]) == "" {
		humanDescription["zh_Hans"] = firstNonBlank(label["zh_Hans"], title)
	}
	placeholder := jsonSchemaI18nText(definition["placeholder_i18n"], "")
	if strings.TrimSpace(placeholder["en_US"]) == "" {
		placeholder["en_US"] = humanDescription["en_US"]
	}
	if strings.TrimSpace(placeholder["zh_Hans"]) == "" {
		placeholder["zh_Hans"] = humanDescription["zh_Hans"]
	}
	parameterType := jsonSchemaToolParameterType(definition)
	options := jsonSchemaToolParameterOptions(definition)
	if len(options) > 0 {
		parameterType = ToolParameterTypeSelect
	}
	parameter := ToolParameter{
		Name:             name,
		Label:            label,
		HumanDescription: humanDescription,
		LLMDescription:   description,
		Type:             parameterType,
		Form:             ToolParameterFormLLM,
		Required:         required,
		Default:          definition["default"],
		Options:          options,
		Placeholder:      placeholder,
		SupportVariable:  true,
	}
	parameter.MinValue = jsonSchemaNumberPointer(definition["minimum"])
	parameter.MaxValue = jsonSchemaNumberPointer(definition["maximum"])
	return parameter, nil
}

func jsonSchemaToolParameterType(definition map[string]interface{}) ToolParameterType {
	schemaType := strings.ToLower(strings.TrimSpace(jsonSchemaTypeName(definition["type"])))
	switch schemaType {
	case "string":
		switch strings.ToLower(strings.TrimSpace(jsonSchemaString(definition["format"]))) {
		case "date":
			return ToolParameterTypeDate
		case "date-time":
			return ToolParameterTypeDateTime
		}
		return ToolParameterTypeString
	case "integer", "number":
		return ToolParameterTypeNumber
	case "boolean":
		return ToolParameterTypeBoolean
	case "array":
		if items, ok := definition["items"].(map[string]interface{}); ok && strings.EqualFold(jsonSchemaTypeName(items["type"]), "string") {
			return ToolParameterTypeStringList
		}
		return ToolParameterTypeJSON
	case "object":
		return ToolParameterTypeJSON
	default:
		// Conditional and union schemas cannot be represented by the legacy
		// primitive form contract. A JSON editor preserves their value shape.
		return ToolParameterTypeJSON
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func jsonSchemaToolParameterOptions(definition map[string]interface{}) []ToolParameterOption {
	values := jsonSchemaStringSlice(definition["enum"])
	if len(values) == 0 {
		return nil
	}
	options := make([]ToolParameterOption, 0, len(values))
	for _, value := range values {
		options = append(options, ToolParameterOption{
			Value: value,
			Label: jsonSchemaEnumLabel(definition["enum_labels_i18n"], value),
		})
	}
	return options
}

func jsonSchemaI18nText(raw interface{}, fallback string) I18nText {
	out := I18nText{}
	for locale, value := range jsonSchemaStringMap(raw) {
		if strings.TrimSpace(value) == "" {
			continue
		}
		switch strings.ToLower(strings.ReplaceAll(locale, "_", "-")) {
		case "en", "en-us":
			out["en_US"] = value
		case "zh", "zh-cn", "zh-hans":
			out["zh_Hans"] = value
		}
	}
	if _, exists := out["en_US"]; !exists && strings.TrimSpace(fallback) != "" {
		out["en_US"] = strings.TrimSpace(fallback)
	}
	if _, exists := out["zh_Hans"]; !exists && strings.TrimSpace(fallback) != "" {
		out["zh_Hans"] = strings.TrimSpace(fallback)
	}
	return out
}

func jsonSchemaEnumLabel(raw interface{}, value string) I18nText {
	out := I18nText{"en_US": value, "zh_Hans": value}
	for locale, rawLabels := range jsonSchemaMap(raw) {
		label := jsonSchemaStringMap(rawLabels)[value]
		if strings.TrimSpace(label) == "" {
			continue
		}
		switch strings.ToLower(strings.ReplaceAll(locale, "_", "-")) {
		case "en", "en-us":
			out["en_US"] = label
		case "zh", "zh-cn", "zh-hans":
			out["zh_Hans"] = label
		}
	}
	return out
}

func jsonSchemaStringSlice(raw interface{}) []string {
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

// Provider definitions use both plain maps and named/strongly typed maps for
// localized schema metadata. Reflection is kept at this catalog-conversion
// boundary so the tools package does not need to import the integrations
// package and create a dependency cycle.
func jsonSchemaMap(raw interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	value := reflect.ValueOf(raw)
	if !value.IsValid() || value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
		return out
	}
	iterator := value.MapRange()
	for iterator.Next() {
		out[iterator.Key().String()] = iterator.Value().Interface()
	}
	return out
}

func jsonSchemaStringMap(raw interface{}) map[string]string {
	out := map[string]string{}
	for key, value := range jsonSchemaMap(raw) {
		text, ok := value.(string)
		if ok {
			out[key] = text
		}
	}
	return out
}

func jsonSchemaStringSet(raw interface{}) map[string]bool {
	out := map[string]bool{}
	switch values := raw.(type) {
	case []interface{}:
		for _, value := range values {
			if name, ok := value.(string); ok && strings.TrimSpace(name) != "" {
				out[name] = true
			}
		}
	case []string:
		for _, name := range values {
			if strings.TrimSpace(name) != "" {
				out[name] = true
			}
		}
	}
	return out
}

func jsonSchemaTypeName(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	case []interface{}:
		for _, candidate := range value {
			if name, ok := candidate.(string); ok && name != "null" {
				return name
			}
		}
	}
	return ""
}

func jsonSchemaString(raw interface{}) string {
	value, _ := raw.(string)
	return value
}

func jsonSchemaNumberPointer(raw interface{}) *float64 {
	var value float64
	switch typed := raw.(type) {
	case float64:
		value = typed
	case float32:
		value = float64(typed)
	case int:
		value = float64(typed)
	case int64:
		value = float64(typed)
	default:
		return nil
	}
	return &value
}
