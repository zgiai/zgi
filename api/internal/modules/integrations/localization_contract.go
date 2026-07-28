package integrations

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	maxLocalizedInputSchemaDepth  = 32
	maxLocalizedInputSchemaFields = 2048
)

var requiredCatalogLocales = []string{LocaleEnglishUS, LocaleSimplifiedChinese}

// validateProviderLocalizationContract prevents a provider from being
// registered when a product-supported language would have to fall back to an
// identifier or to copy intended for another language. Output schemas are not
// included: they are machine/model result contracts and are not rendered as
// provider-authored form labels.
func validateProviderLocalizationContract(definition ProviderDefinition) error {
	if err := requireSupportedLocalizedText("provider.name_i18n", definition.NameI18n); err != nil {
		return err
	}
	if err := requireSupportedLocalizedText("provider.description_i18n", definition.DescriptionI18n); err != nil {
		return err
	}
	if err := requireDeclaredLocalizedLabels("provider.tag_labels_i18n", definition.Tags, definition.TagLabelsI18n); err != nil {
		return err
	}
	if err := requireDeclaredLocalizedLabels("provider.category_labels_i18n", definition.Categories, definition.CategoryLabelsI18n); err != nil {
		return err
	}
	for _, scope := range definition.Scopes {
		scopePath := fmt.Sprintf("provider.scopes[%s]", scope.ID)
		if err := requireSupportedLocalizedText(scopePath+".label_i18n", scope.LabelI18n); err != nil {
			return err
		}
		if hasEnglishLocalizedContent(scope.Description, scope.DescriptionI18n) {
			if err := requireSupportedLocalizedText(scopePath+".description_i18n", scope.DescriptionI18n); err != nil {
				return err
			}
		}
	}

	for _, method := range definition.AuthMethods {
		methodPath := fmt.Sprintf("provider.auth[%s]", method.ID)
		if err := requireSupportedLocalizedText(methodPath+".label_i18n", method.LabelI18n); err != nil {
			return err
		}
		if hasEnglishLocalizedContent(method.Description, method.DescriptionI18n) {
			if err := requireSupportedLocalizedText(methodPath+".description_i18n", method.DescriptionI18n); err != nil {
				return err
			}
		}
		for _, field := range method.Fields {
			fieldPath := fmt.Sprintf("%s.fields[%s]", methodPath, field.Key)
			if err := requireSupportedLocalizedText(fieldPath+".label_i18n", field.LabelI18n); err != nil {
				return err
			}
			if hasEnglishLocalizedContent(field.Description, field.DescriptionI18n) {
				if err := requireSupportedLocalizedText(fieldPath+".description_i18n", field.DescriptionI18n); err != nil {
					return err
				}
			}
			if hasEnglishLocalizedContent(field.Placeholder, field.PlaceholderI18n) {
				if err := requireSupportedLocalizedText(fieldPath+".placeholder_i18n", field.PlaceholderI18n); err != nil {
					return err
				}
			}
			for _, option := range field.Options {
				optionPath := fmt.Sprintf("%s.options[%s].label_i18n", fieldPath, option.Value)
				if err := requireSupportedLocalizedText(optionPath, option.LabelI18n); err != nil {
					return err
				}
			}
		}
		if method.OAuth != nil {
			for _, field := range method.OAuth.ClientFields {
				fieldPath := fmt.Sprintf("%s.oauth.client_fields[%s]", methodPath, field.Key)
				if err := requireSupportedLocalizedText(fieldPath+".label_i18n", field.LabelI18n); err != nil {
					return err
				}
				if hasEnglishLocalizedContent(field.Description, field.DescriptionI18n) {
					if err := requireSupportedLocalizedText(fieldPath+".description_i18n", field.DescriptionI18n); err != nil {
						return err
					}
				}
			}
		}
	}

	if hasEnglishLocalizedContent(definition.HealthProbe.Description, definition.HealthProbe.DescriptionI18n) {
		if err := requireSupportedLocalizedText("provider.health_probe.description_i18n", definition.HealthProbe.DescriptionI18n); err != nil {
			return err
		}
	}

	for _, action := range definition.Actions {
		actionPath := fmt.Sprintf("provider.actions[%s]", action.ID)
		if err := requireSupportedLocalizedText(actionPath+".name_i18n", action.NameI18n); err != nil {
			return err
		}
		if err := requireSupportedLocalizedText(actionPath+".description_i18n", action.DescriptionI18n); err != nil {
			return err
		}
		if err := requireDeclaredLocalizedLabels(actionPath+".scope_labels_i18n", ActionRequiredScopeIDs(action), action.ScopeLabelsI18n); err != nil {
			return err
		}
		if err := validateActionInputSchemaLocalization(actionPath+".input_schema", action.InputSchema); err != nil {
			return err
		}
	}
	return nil
}

func requireSupportedLocalizedText(path string, values LocalizedText) error {
	for _, locale := range requiredCatalogLocales {
		if strings.TrimSpace(values[locale]) == "" {
			return fmt.Errorf("%s.%s is required", path, locale)
		}
	}
	return nil
}

func hasEnglishLocalizedContent(fallback string, values LocalizedText) bool {
	return strings.TrimSpace(fallback) != "" || strings.TrimSpace(values[LocaleEnglishUS]) != ""
}

func requireDeclaredLocalizedLabels(path string, declared []string, labels LocalizedLabelMap) error {
	for _, identifier := range declared {
		localized, exists := labels[identifier]
		if !exists {
			return fmt.Errorf("%s.%s is required", path, identifier)
		}
		if err := requireSupportedLocalizedText(path+"."+identifier, localized); err != nil {
			return err
		}
	}
	return nil
}

type inputSchemaLocalizationState struct {
	fields int
}

func validateActionInputSchemaLocalization(path string, schema map[string]interface{}) error {
	state := &inputSchemaLocalizationState{}
	return validateInputSchemaNode(path, schema, false, 0, state)
}

func validateInputSchemaNode(path string, schema map[string]interface{}, namedProperty bool, depth int, state *inputSchemaLocalizationState) error {
	if depth > maxLocalizedInputSchemaDepth {
		return fmt.Errorf("%s exceeds the localization depth limit", path)
	}
	if namedProperty {
		state.fields++
		if state.fields > maxLocalizedInputSchemaFields {
			return fmt.Errorf("%s exceeds the localization field limit", path)
		}
		if err := requireSupportedAnnotation(path+".title_i18n", schema["title_i18n"]); err != nil {
			return err
		}
	}
	if rawEnum, exists := schema["enum"]; exists {
		values, err := enumLocalizationKeys(rawEnum)
		if err != nil {
			return fmt.Errorf("%s.enum: %w", path, err)
		}
		if len(values) > 0 {
			if err := requireSupportedEnumLabels(path+".enum_labels_i18n", schema["enum_labels_i18n"], values); err != nil {
				return err
			}
		}
	}

	properties, err := schemaObject(schema["properties"])
	if err != nil {
		return fmt.Errorf("%s.properties: %w", path, err)
	}
	for _, propertyName := range sortedSchemaKeys(properties) {
		property, err := schemaObject(properties[propertyName])
		if err != nil {
			return fmt.Errorf("%s.properties.%s: %w", path, propertyName, err)
		}
		if err := validateInputSchemaNode(path+".properties."+propertyName, property, true, depth+1, state); err != nil {
			return err
		}
	}

	for _, keyword := range []string{"items", "contains", "additionalProperties", "unevaluatedProperties", "propertyNames", "not", "if", "then", "else"} {
		child, exists := schema[keyword]
		if !exists {
			continue
		}
		childSchema, ok := child.(map[string]interface{})
		if !ok {
			// Boolean schemas are valid and contain no user-facing fields.
			if _, booleanSchema := child.(bool); booleanSchema {
				continue
			}
			return fmt.Errorf("%s.%s must be a schema object", path, keyword)
		}
		if err := validateInputSchemaNode(path+"."+keyword, childSchema, false, depth+1, state); err != nil {
			return err
		}
	}

	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		children, exists := schema[keyword]
		if !exists {
			continue
		}
		list, ok := interfaceSlice(children)
		if !ok {
			return fmt.Errorf("%s.%s must be a schema array", path, keyword)
		}
		for index, child := range list {
			childSchema, ok := child.(map[string]interface{})
			if !ok {
				return fmt.Errorf("%s.%s[%d] must be a schema object", path, keyword, index)
			}
			if err := validateInputSchemaNode(fmt.Sprintf("%s.%s[%d]", path, keyword, index), childSchema, false, depth+1, state); err != nil {
				return err
			}
		}
	}

	for _, keyword := range []string{"$defs", "definitions", "patternProperties", "dependentSchemas"} {
		children, err := schemaObject(schema[keyword])
		if err != nil {
			return fmt.Errorf("%s.%s: %w", path, keyword, err)
		}
		for _, name := range sortedSchemaKeys(children) {
			childSchema, err := schemaObject(children[name])
			if err != nil {
				return fmt.Errorf("%s.%s.%s: %w", path, keyword, name, err)
			}
			if err := validateInputSchemaNode(path+"."+keyword+"."+name, childSchema, false, depth+1, state); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireSupportedAnnotation(path string, raw interface{}) error {
	values, err := annotationStringMap(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, locale := range requiredCatalogLocales {
		value, exists := values[locale]
		if !exists || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s.%s is required", path, locale)
		}
	}
	return nil
}

func requireSupportedEnumLabels(path string, raw interface{}, values []string) error {
	localized, err := annotationObject(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, locale := range requiredCatalogLocales {
		localeLabels, exists := localized[locale]
		if !exists {
			return fmt.Errorf("%s.%s is required", path, locale)
		}
		labels, err := annotationStringMap(localeLabels)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", path, locale, err)
		}
		for _, value := range values {
			if strings.TrimSpace(labels[value]) == "" {
				return fmt.Errorf("%s.%s.%s is required", path, locale, value)
			}
		}
	}
	return nil
}

func annotationStringMap(raw interface{}) (map[string]string, error) {
	values, err := annotationObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(values))
	for key, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be a non-empty string", key)
		}
		out[key] = value
	}
	return out, nil
}

func annotationObject(raw interface{}) (map[string]interface{}, error) {
	switch typed := raw.(type) {
	case nil:
		return nil, nil
	case map[string]interface{}:
		return typed, nil
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out, nil
	case LocalizedText:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out, nil
	case map[string]map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out, nil
	case map[string]map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out, nil
	case map[string]LocalizedText:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be an object")
	}
}

func schemaObject(raw interface{}) (map[string]interface{}, error) {
	if raw == nil {
		return nil, nil
	}
	value, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	return value, nil
}

func enumLocalizationKeys(raw interface{}) ([]string, error) {
	values, ok := interfaceSlice(raw)
	if !ok {
		return nil, fmt.Errorf("must be an array")
	}
	keys := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			keys = append(keys, text)
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("contains an unsupported value")
		}
		keys = append(keys, string(encoded))
	}
	return keys, nil
}

func interfaceSlice(raw interface{}) ([]interface{}, bool) {
	if raw == nil {
		return nil, false
	}
	value := reflect.ValueOf(raw)
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]interface{}, value.Len())
	for index := 0; index < value.Len(); index++ {
		out[index] = value.Index(index).Interface()
	}
	return out, true
}

func sortedSchemaKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
