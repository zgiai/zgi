package providers

import (
	"encoding/json"
	"strconv"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

type operationSchema struct {
	input, output *jsonschema.Schema
	defaults      map[string]any
}
type Schemas struct{ operations map[string]operationSchema }

func NewSchemas(connectorID string) (*Schemas, error) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		return nil, err
	}
	return newSchemas(connectorID, definitions)
}

func NewSchemasForDefinition(definition catalog.Definition) (*Schemas, error) {
	if err := catalog.Validate(definition); err != nil {
		return nil, err
	}
	return newSchemas(definition.ID, []catalog.Definition{definition})
}

func newSchemas(connectorID string, definitions []catalog.Definition) (*Schemas, error) {
	var err error
	schemas := &Schemas{operations: map[string]operationSchema{}}
	for _, definition := range definitions {
		if definition.ID != connectorID {
			continue
		}
		for _, operation := range definition.Operations {
			compiler := jsonschema.NewCompiler()
			compiler.DefaultDraft(jsonschema.Draft2020)
			if err = compiler.AddResource("urn:input", operation.InputSchema); err != nil {
				return nil, err
			}
			if err = compiler.AddResource("urn:output", operation.OutputSchema); err != nil {
				return nil, err
			}
			input, err := compiler.Compile("urn:input")
			if err != nil {
				return nil, err
			}
			output, err := compiler.Compile("urn:output")
			if err != nil {
				return nil, err
			}
			defaults := map[string]any{}
			properties, _ := operation.InputSchema["properties"].(map[string]any)
			for key, raw := range properties {
				if property, ok := raw.(map[string]any); ok {
					if value, exists := property["default"]; exists {
						defaults[key] = value
					}
				}
			}
			schemas.operations[operation.ID] = operationSchema{input: input, output: output, defaults: defaults}
		}
	}
	if len(schemas.operations) == 0 {
		return nil, Invalid()
	}
	return schemas, nil
}
func (schemas *Schemas) Input(operation string, input map[string]any) (map[string]any, error) {
	contract, ok := schemas.operations[operation]
	if !ok {
		return nil, Invalid()
	}
	if input == nil {
		input = map[string]any{}
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, Invalid()
	}
	var normalized map[string]any
	if json.Unmarshal(data, &normalized) != nil || contract.input.Validate(normalized) != nil {
		return nil, Invalid()
	}
	for key, value := range contract.defaults {
		if _, ok := normalized[key]; !ok {
			normalized[key] = value
		}
	}
	return normalized, nil
}
func (schemas *Schemas) Output(operation string, value map[string]any) error {
	contract, ok := schemas.operations[operation]
	if !ok {
		return Invalid()
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return Invalid()
	}
	var normalized any
	if json.Unmarshal(encoded, &normalized) != nil || contract.output.Validate(normalized) != nil {
		return Invalid()
	}
	return nil
}
func Text(input map[string]any, key string) string { value, _ := input[key].(string); return value }
func Number(input map[string]any, key string) int {
	switch value := input[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		n, _ := strconv.Atoi(string(value))
		return n
	}
	return 0
}
func TextList(input map[string]any, key string) []string {
	values := []string{}
	switch list := input[key].(type) {
	case []any:
		for _, raw := range list {
			if text, ok := raw.(string); ok {
				values = append(values, text)
			}
		}
	case []string:
		values = append(values, list...)
	}
	return values
}
