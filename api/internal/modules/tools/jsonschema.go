package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	jsonschemakind "github.com/santhosh-tekuri/jsonschema/v6/kind"
)

var compiledJSONSchemas sync.Map

const maxJSONSchemaValidationIssues = 8

const (
	maxJSONSchemaFeedbackDepth      = 8
	maxJSONSchemaFeedbackProperties = 64
	maxJSONSchemaFeedbackBranches   = 16
	maxJSONSchemaFeedbackPathLength = 256
	maxJSONSchemaProjectionDepth    = 64
)

// JSONSchemaValidationIssue is a safe, value-free description of one schema
// violation. Path components are limited to property names declared by the
// schema (plus array indexes), so attacker-controlled keys and values cannot be
// reflected into model feedback, traces, or logs.
type JSONSchemaValidationIssue struct {
	Path     string `json:"path"`
	Keyword  string `json:"keyword"`
	Expected string `json:"expected"`
}

// ValidateJSONSchema verifies that schema is a valid Draft 2020-12 schema.
// An empty schema is allowed for compatibility with existing tools.
func ValidateJSONSchema(schema map[string]interface{}) error {
	if len(schema) == 0 {
		return nil
	}
	_, err := compileJSONSchema(schema)
	return err
}

// ValidateJSONSchemaValue validates a value against a Draft 2020-12 schema.
// An empty schema intentionally accepts every value for legacy tools.
func ValidateJSONSchemaValue(schema map[string]interface{}, value interface{}) error {
	if len(schema) == 0 {
		return nil
	}
	compiled, err := compileJSONSchema(schema)
	if err != nil {
		return err
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("json schema validation failed: %w", err)
	}
	return nil
}

// SafeJSONSchemaValidationIssues converts a validator error into bounded,
// structural feedback without including the rejected value. The schema is
// required so instance paths can be restricted to trusted property names.
func SafeJSONSchemaValidationIssues(schema map[string]interface{}, err error) []JSONSchemaValidationIssue {
	if err == nil || len(schema) == 0 {
		return nil
	}
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) || validationErr == nil {
		return nil
	}

	visibleSchema := ModelVisibleJSONSchema(schema)
	if len(visibleSchema) == 0 {
		return nil
	}
	issues := make([]JSONSchemaValidationIssue, 0, 2)
	collectSafeJSONSchemaValidationIssues(validationErr, visibleSchema, &issues)
	if len(issues) > maxJSONSchemaValidationIssues {
		issues = issues[:maxJSONSchemaValidationIssues]
	}
	return issues
}

// SafeJSONSchemaForFeedback returns a bounded structural subset of a provider
// schema for model retry guidance. Free-form annotations, examples, defaults,
// constants, patterns, UUID enum values, extension fields, and unknown
// keywords are deliberately omitted because future dynamic providers may be
// untrusted.
func SafeJSONSchemaForFeedback(schema map[string]interface{}) map[string]interface{} {
	safe, ok := safeJSONSchemaFeedbackValue(ModelVisibleJSONSchema(schema), 0, "")
	if !ok {
		return nil
	}
	result, _ := safe.(map[string]interface{})
	return result
}

// ModelVisibleJSONSchema returns a deep-cloned schema with server-owned
// readOnly properties removed. The source schema is never mutated and remains
// authoritative for post-enrichment execution validation.
func ModelVisibleJSONSchema(schema map[string]interface{}) map[string]interface{} {
	if len(schema) == 0 {
		return nil
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var normalized map[string]interface{}
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil
	}
	projected, ok := modelVisibleJSONSchemaValue(normalized, nil).(map[string]interface{})
	if !ok || len(projected) == 0 {
		return nil
	}
	if err := ValidateJSONSchema(projected); err != nil {
		return nil
	}
	return projected
}

func collectSafeJSONSchemaValidationIssues(validationErr *jsonschema.ValidationError, visibleSchema map[string]interface{}, issues *[]JSONSchemaValidationIssue) {
	if validationErr == nil || len(*issues) >= maxJSONSchemaValidationIssues {
		return
	}
	if required, ok := validationErr.ErrorKind.(*jsonschemakind.Required); ok {
		for _, missing := range required.Missing {
			if len(*issues) >= maxJSONSchemaValidationIssues {
				return
			}
			parentSchema, trustedPath := modelVisibleJSONSchemaAtInstanceLocation(visibleSchema, validationErr.InstanceLocation)
			if !trustedPath {
				continue
			}
			if _, trusted := modelVisibleJSONSchemaProperty(parentSchema, missing, 0); !trusted {
				continue
			}
			path, trustedPath := safeJSONSchemaInstancePath(validationErr.InstanceLocation, visibleSchema)
			if !trustedPath {
				continue
			}
			if path == "$" {
				path = missing
			} else {
				path += "." + missing
			}
			appendUniqueJSONSchemaValidationIssue(issues, JSONSchemaValidationIssue{
				Path:     path,
				Keyword:  "required",
				Expected: "required field",
			})
		}
	}

	if len(validationErr.Causes) > 0 {
		for _, cause := range validationErr.Causes {
			collectSafeJSONSchemaValidationIssues(cause, visibleSchema, issues)
		}
		return
	}
	if validationErr.ErrorKind == nil {
		return
	}
	keywordPath := validationErr.ErrorKind.KeywordPath()
	if len(keywordPath) == 0 {
		return
	}
	keyword := strings.TrimSpace(keywordPath[0])
	if keyword == "" || keyword == "required" || !isAllowedJSONSchemaFeedbackKeyword(keyword) {
		return
	}
	path, trustedPath := safeJSONSchemaInstancePath(validationErr.InstanceLocation, visibleSchema)
	if !trustedPath {
		return
	}
	appendUniqueJSONSchemaValidationIssue(issues, JSONSchemaValidationIssue{
		Path:     path,
		Keyword:  keyword,
		Expected: safeJSONSchemaExpectation(validationErr.ErrorKind, keyword),
	})
}

func appendUniqueJSONSchemaValidationIssue(issues *[]JSONSchemaValidationIssue, issue JSONSchemaValidationIssue) {
	if strings.TrimSpace(issue.Path) == "" {
		issue.Path = "$"
	}
	if strings.TrimSpace(issue.Expected) == "" {
		issue.Expected = "value matching the schema constraint"
	}
	for _, existing := range *issues {
		if existing == issue {
			return
		}
	}
	*issues = append(*issues, issue)
}

func safeJSONSchemaExpectation(kind jsonschema.ErrorKind, keyword string) string {
	switch typed := kind.(type) {
	case *jsonschemakind.Type:
		if len(typed.Want) > 0 {
			return "type " + strings.Join(typed.Want, " or ")
		}
	case *jsonschemakind.Format:
		format := strings.TrimSpace(typed.Want)
		if isSafeJSONSchemaLabel(format) {
			return "format " + format
		}
		return "declared string format"
	case *jsonschemakind.MinProperties:
		return "at least " + strconv.Itoa(typed.Want) + " properties"
	case *jsonschemakind.MaxProperties:
		return "at most " + strconv.Itoa(typed.Want) + " properties"
	case *jsonschemakind.MinItems:
		return "at least " + strconv.Itoa(typed.Want) + " items"
	case *jsonschemakind.MaxItems:
		return "at most " + strconv.Itoa(typed.Want) + " items"
	case *jsonschemakind.MinLength:
		return "minimum length " + strconv.Itoa(typed.Want)
	case *jsonschemakind.MaxLength:
		return "maximum length " + strconv.Itoa(typed.Want)
	case *jsonschemakind.AdditionalProperties:
		return "no additional properties"
	case *jsonschemakind.Enum:
		return "one of the allowed values"
	case *jsonschemakind.Const:
		return "the declared constant"
	case *jsonschemakind.Pattern:
		return "string matching the declared pattern"
	}
	if isSafeJSONSchemaLabel(keyword) {
		return "constraint " + keyword
	}
	return "value matching the schema constraint"
}

func safeJSONSchemaInstancePath(instanceLocation []string, visibleSchema map[string]interface{}) (string, bool) {
	if len(instanceLocation) == 0 {
		return "$", true
	}
	var path strings.Builder
	current := visibleSchema
	for _, segment := range instanceLocation {
		if index, err := strconv.Atoi(segment); err == nil && index >= 0 && index <= 9999 {
			itemSchema, ok := modelVisibleJSONSchemaArrayItem(current, index)
			if !ok {
				return "$", false
			}
			if path.Len()+len(segment)+2 > maxJSONSchemaFeedbackPathLength {
				break
			}
			path.WriteString("[")
			path.WriteString(segment)
			path.WriteString("]")
			current = itemSchema
			continue
		}
		if !isSafeJSONSchemaLabel(segment) {
			return "$", false
		}
		propertySchema, trusted := modelVisibleJSONSchemaProperty(current, segment, 0)
		if !trusted {
			return "$", false
		}
		separatorLength := 0
		if path.Len() > 0 {
			separatorLength = 1
		}
		if path.Len()+separatorLength+len(segment) > maxJSONSchemaFeedbackPathLength {
			break
		}
		if path.Len() > 0 {
			path.WriteString(".")
		}
		path.WriteString(segment)
		current = propertySchema
	}
	if path.Len() == 0 {
		return "$", false
	}
	return path.String(), true
}

func modelVisibleJSONSchemaAtInstanceLocation(schema map[string]interface{}, instanceLocation []string) (map[string]interface{}, bool) {
	current := schema
	for _, segment := range instanceLocation {
		if index, err := strconv.Atoi(segment); err == nil && index >= 0 && index <= 9999 {
			next, ok := modelVisibleJSONSchemaArrayItem(current, index)
			if !ok {
				return nil, false
			}
			current = next
			continue
		}
		next, ok := modelVisibleJSONSchemaProperty(current, segment, 0)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func modelVisibleJSONSchemaProperty(schema map[string]interface{}, name string, depth int) (map[string]interface{}, bool) {
	if schema == nil || depth > maxJSONSchemaFeedbackDepth || !isSafeJSONSchemaLabel(name) {
		return nil, false
	}
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		if property, exists := properties[name]; exists {
			if propertySchema, ok := property.(map[string]interface{}); ok {
				return propertySchema, true
			}
			return map[string]interface{}{}, true
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		if branches, ok := schema[keyword].([]interface{}); ok {
			for index, branch := range branches {
				if index >= maxJSONSchemaFeedbackBranches {
					break
				}
				if branchSchema, ok := branch.(map[string]interface{}); ok {
					if property, found := modelVisibleJSONSchemaProperty(branchSchema, name, depth+1); found {
						return property, true
					}
				}
			}
		}
	}
	return nil, false
}

func modelVisibleJSONSchemaArrayItem(schema map[string]interface{}, index int) (map[string]interface{}, bool) {
	if schema == nil || index < 0 {
		return nil, false
	}
	if prefixItems, ok := schema["prefixItems"].([]interface{}); ok && index < len(prefixItems) {
		if item, ok := prefixItems[index].(map[string]interface{}); ok {
			return item, true
		}
		return map[string]interface{}{}, true
	}
	if item, ok := schema["items"].(map[string]interface{}); ok {
		return item, true
	}
	return nil, false
}

func modelVisibleJSONSchemaValue(value interface{}, inheritedHidden map[string]struct{}) interface{} {
	return modelVisibleJSONSchemaValueAtDepth(value, inheritedHidden, 0)
}

func modelVisibleJSONSchemaValueAtDepth(value interface{}, inheritedHidden map[string]struct{}, depth int) interface{} {
	if depth > maxJSONSchemaProjectionDepth {
		return nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		hidden := copyJSONSchemaPropertySet(inheritedHidden)
		collectSameScopeReadOnlyJSONSchemaProperties(typed, hidden, depth)
		out := make(map[string]interface{}, len(typed))
		hiddenConditional := schemaReferencesHiddenJSONSchemaProperty(typed["if"], hidden)
		for key, child := range typed {
			switch key {
			case "readOnly":
				continue
			case "x-zgi-discard-when":
				// This is a server-only input canonicalization rule. Provider
				// function schemas must not expose private orchestration keywords.
				continue
			case "properties":
				properties, ok := child.(map[string]interface{})
				if !ok {
					continue
				}
				visibleProperties := make(map[string]interface{}, len(properties))
				for name, definition := range properties {
					if _, hiddenProperty := hidden[name]; hiddenProperty {
						continue
					}
					// A property value is a new instance scope. It must not inherit
					// hidden names from the containing object.
					visibleProperties[name] = modelVisibleJSONSchemaValueAtDepth(definition, nil, depth+1)
				}
				out[key] = visibleProperties
			case "required":
				if required := modelVisibleJSONSchemaStringList(child, hidden); len(required) > 0 {
					out[key] = required
				}
			case "dependentRequired":
				if dependencies := modelVisibleDependentRequired(child, hidden); len(dependencies) > 0 {
					out[key] = dependencies
				}
			case "dependencies":
				if dependencies := modelVisibleDependencies(child, hidden, depth+1); len(dependencies) > 0 {
					out[key] = dependencies
				}
			case "dependentSchemas":
				if dependencies := modelVisibleDependentSchemas(child, hidden, depth+1); len(dependencies) > 0 {
					out[key] = dependencies
				}
			case "not":
				// A negated condition involving a server-owned field becomes
				// vacuously true for model input; omit it instead of narrowing
				// the remaining visible condition incorrectly.
				if !schemaReferencesHiddenJSONSchemaProperty(child, hidden) {
					out[key] = modelVisibleJSONSchemaValueAtDepth(child, hidden, depth+1)
				}
			case "if", "then", "else":
				if !hiddenConditional {
					out[key] = modelVisibleJSONSchemaValueAtDepth(child, hidden, depth+1)
				}
			case "allOf", "anyOf", "oneOf":
				out[key] = modelVisibleJSONSchemaValueAtDepth(child, hidden, depth+1)
			case "items", "prefixItems", "contains", "additionalProperties", "unevaluatedProperties",
				"propertyNames", "patternProperties", "$defs", "definitions":
				// These schemas validate nested values or reusable definitions and
				// therefore start their own property-name scope.
				out[key] = modelVisibleJSONSchemaValueAtDepth(child, nil, depth+1)
			default:
				out[key] = modelVisibleJSONSchemaValueAtDepth(child, nil, depth+1)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, child := range typed {
			projected := modelVisibleJSONSchemaValueAtDepth(child, inheritedHidden, depth+1)
			if projected != nil {
				out = append(out, projected)
			}
		}
		return out
	default:
		return typed
	}
}

func copyJSONSchemaPropertySet(values map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for value := range values {
		out[value] = struct{}{}
	}
	return out
}

func collectSameScopeReadOnlyJSONSchemaProperties(schema map[string]interface{}, hidden map[string]struct{}, depth int) {
	if schema == nil || depth > maxJSONSchemaProjectionDepth {
		return
	}
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for name, definition := range properties {
			if propertySchema, ok := definition.(map[string]interface{}); ok {
				if readOnly, _ := propertySchema["readOnly"].(bool); readOnly {
					hidden[name] = struct{}{}
				}
			}
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		if branches, ok := schema[keyword].([]interface{}); ok {
			for _, branch := range branches {
				if branchSchema, ok := branch.(map[string]interface{}); ok {
					collectSameScopeReadOnlyJSONSchemaProperties(branchSchema, hidden, depth+1)
				}
			}
		}
	}
	for _, keyword := range []string{"not", "if", "then", "else"} {
		if branch, ok := schema[keyword].(map[string]interface{}); ok {
			collectSameScopeReadOnlyJSONSchemaProperties(branch, hidden, depth+1)
		}
	}
	for _, keyword := range []string{"dependentSchemas", "dependencies"} {
		if dependencies, ok := schema[keyword].(map[string]interface{}); ok {
			for _, dependency := range dependencies {
				if dependencySchema, ok := dependency.(map[string]interface{}); ok {
					collectSameScopeReadOnlyJSONSchemaProperties(dependencySchema, hidden, depth+1)
				}
			}
		}
	}
}

func modelVisibleJSONSchemaStringList(value interface{}, hidden map[string]struct{}) []interface{} {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]interface{}, 0, len(values))
	for _, item := range values {
		name, ok := item.(string)
		if !ok {
			continue
		}
		if _, hiddenProperty := hidden[name]; hiddenProperty {
			continue
		}
		out = append(out, name)
	}
	return out
}

func modelVisibleDependentRequired(value interface{}, hidden map[string]struct{}) map[string]interface{} {
	dependencies, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]interface{}, len(dependencies))
	for property, raw := range dependencies {
		if _, hiddenProperty := hidden[property]; hiddenProperty {
			continue
		}
		if required := modelVisibleJSONSchemaStringList(raw, hidden); len(required) > 0 {
			out[property] = required
		}
	}
	return out
}

func modelVisibleDependencies(value interface{}, hidden map[string]struct{}, depth int) map[string]interface{} {
	dependencies, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]interface{}, len(dependencies))
	for property, raw := range dependencies {
		if _, hiddenProperty := hidden[property]; hiddenProperty {
			continue
		}
		if required, ok := raw.([]interface{}); ok {
			if visible := modelVisibleJSONSchemaStringList(required, hidden); len(visible) > 0 {
				out[property] = visible
			}
			continue
		}
		if projected := modelVisibleJSONSchemaValueAtDepth(raw, hidden, depth+1); projected != nil {
			out[property] = projected
		}
	}
	return out
}

func modelVisibleDependentSchemas(value interface{}, hidden map[string]struct{}, depth int) map[string]interface{} {
	dependencies, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]interface{}, len(dependencies))
	for property, raw := range dependencies {
		if _, hiddenProperty := hidden[property]; hiddenProperty {
			continue
		}
		if projected := modelVisibleJSONSchemaValueAtDepth(raw, hidden, depth+1); projected != nil {
			out[property] = projected
		}
	}
	return out
}

func schemaReferencesHiddenJSONSchemaProperty(value interface{}, hidden map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			switch key {
			case "properties", "dependentRequired", "dependencies", "dependentSchemas":
				if entries, ok := child.(map[string]interface{}); ok {
					for property, entry := range entries {
						if _, hiddenProperty := hidden[property]; hiddenProperty {
							return true
						}
						if schemaReferencesHiddenJSONSchemaProperty(entry, hidden) {
							return true
						}
					}
				}
			case "required":
				if required, ok := child.([]interface{}); ok {
					for _, item := range required {
						if name, ok := item.(string); ok {
							if _, hiddenProperty := hidden[name]; hiddenProperty {
								return true
							}
						}
					}
				}
			default:
				if schemaReferencesHiddenJSONSchemaProperty(child, hidden) {
					return true
				}
			}
		}
	case []interface{}:
		for _, child := range typed {
			if schemaReferencesHiddenJSONSchemaProperty(child, hidden) {
				return true
			}
		}
	}
	return false
}

func safeJSONSchemaFeedbackValue(value interface{}, depth int, propertyName string) (interface{}, bool) {
	if depth > maxJSONSchemaFeedbackDepth {
		return nil, false
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{})
		for key, child := range typed {
			switch key {
			case "type":
				if safe, ok := safeJSONSchemaType(child); ok {
					out[key] = safe
				}
			case "properties":
				properties, ok := child.(map[string]interface{})
				if !ok {
					continue
				}
				safeProperties := make(map[string]interface{})
				for name, definition := range properties {
					if len(safeProperties) >= maxJSONSchemaFeedbackProperties || !isSafeJSONSchemaLabel(name) {
						continue
					}
					if safe, ok := safeJSONSchemaFeedbackValue(definition, depth+1, name); ok {
						safeProperties[name] = safe
					}
				}
				out[key] = safeProperties
			case "required":
				if safe := safeJSONSchemaRequired(child); len(safe) > 0 {
					out[key] = safe
				}
			case "items", "contains", "not", "additionalProperties":
				if boolean, ok := child.(bool); ok && key == "additionalProperties" {
					out[key] = boolean
					continue
				}
				if safe, ok := safeJSONSchemaFeedbackValue(child, depth+1, propertyName); ok {
					out[key] = safe
				}
			case "allOf", "anyOf", "oneOf", "prefixItems":
				if safe := safeJSONSchemaBranches(child, depth+1, propertyName); len(safe) > 0 {
					out[key] = safe
				}
			case "format":
				if label, ok := child.(string); ok && isSafeJSONSchemaLabel(label) {
					out[key] = label
				}
			case "enum":
				if safe, ok := safeJSONSchemaEnum(child, propertyName); ok {
					out[key] = safe
				}
			case "minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties",
				"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf":
				if safe, ok := safeJSONSchemaNumber(child); ok {
					out[key] = safe
				}
			case "uniqueItems", "readOnly", "writeOnly":
				if boolean, ok := child.(bool); ok {
					out[key] = boolean
				}
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func safeJSONSchemaType(value interface{}) (interface{}, bool) {
	allowed := map[string]struct{}{
		"null": {}, "boolean": {}, "object": {}, "array": {},
		"number": {}, "integer": {}, "string": {},
	}
	switch typed := value.(type) {
	case string:
		_, ok := allowed[typed]
		return typed, ok
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			name, ok := item.(string)
			if !ok {
				return nil, false
			}
			if _, ok := allowed[name]; !ok {
				return nil, false
			}
			out = append(out, name)
		}
		return out, len(out) > 0
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, name := range typed {
			if _, ok := allowed[name]; !ok {
				return nil, false
			}
			out = append(out, name)
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func safeJSONSchemaRequired(value interface{}) []interface{} {
	out := make([]interface{}, 0)
	add := func(name string) {
		if len(out) < maxJSONSchemaFeedbackProperties && isSafeJSONSchemaLabel(name) {
			out = append(out, name)
		}
	}
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if name, ok := item.(string); ok {
				add(name)
			}
		}
	case []string:
		for _, name := range typed {
			add(name)
		}
	}
	return out
}

func safeJSONSchemaBranches(value interface{}, depth int, propertyName string) []interface{} {
	values := make([]interface{}, 0)
	switch typed := value.(type) {
	case []interface{}:
		values = typed
	case []map[string]interface{}:
		values = make([]interface{}, 0, len(typed))
		for _, branch := range typed {
			values = append(values, branch)
		}
	default:
		return nil
	}
	out := make([]interface{}, 0, len(values))
	for _, branch := range values {
		if len(out) >= maxJSONSchemaFeedbackBranches {
			break
		}
		if safe, ok := safeJSONSchemaFeedbackValue(branch, depth, propertyName); ok {
			out = append(out, safe)
		}
	}
	return out
}

func safeJSONSchemaEnum(value interface{}, propertyName string) ([]interface{}, bool) {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(propertyName)), "_id") {
		return nil, false
	}
	values := make([]interface{}, 0)
	switch typed := value.(type) {
	case []interface{}:
		values = typed
	case []string:
		values = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
	default:
		return nil, false
	}
	if len(values) == 0 || len(values) > 32 {
		return nil, false
	}
	out := make([]interface{}, 0, len(values))
	for _, item := range values {
		switch typed := item.(type) {
		case string:
			if !isSafeJSONSchemaLabel(typed) || looksLikeUUID(typed) {
				return nil, false
			}
			out = append(out, typed)
		case bool, float64, float32, int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64:
			out = append(out, typed)
		default:
			return nil, false
		}
	}
	return out, true
}

func safeJSONSchemaNumber(value interface{}) (interface{}, bool) {
	switch value.(type) {
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return value, true
	default:
		return nil, false
	}
}

func looksLikeUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func isSafeJSONSchemaLabel(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func isAllowedJSONSchemaFeedbackKeyword(keyword string) bool {
	switch keyword {
	case "type", "enum", "const", "format", "minProperties", "maxProperties",
		"minItems", "maxItems", "additionalItems", "additionalProperties",
		"propertyNames", "uniqueItems", "contains", "minContains", "maxContains",
		"minLength", "maxLength", "pattern", "contentEncoding", "contentMediaType",
		"contentSchema", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
		"multipleOf", "dependentRequired", "dependency", "allOf", "anyOf", "oneOf", "not":
		return true
	default:
		return false
	}
}

func compileJSONSchema(schema map[string]interface{}) (*jsonschema.Schema, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal json schema: %w", err)
	}
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:])
	if cached, ok := compiledJSONSchemas.Load(key); ok {
		return cached.(*jsonschema.Schema), nil
	}

	var document interface{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode json schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	resource := "urn:zgi:tool-schema:" + key
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, fmt.Errorf("add json schema resource: %w", err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compile json schema: %w", err)
	}
	actual, _ := compiledJSONSchemas.LoadOrStore(key, compiled)
	return actual.(*jsonschema.Schema), nil
}
