package tools_test

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// TestWorkflowConnectorSchemasProjectToUsableForms protects the shared
// connector-to-Workflow form boundary. Provider authors keep defining JSON
// Schema, while the Workflow editor must preserve required fields and semantic
// controls instead of degrading dates and string lists to raw text/JSON.
func TestWorkflowConnectorSchemasProjectToUsableForms(t *testing.T) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}

	workflowActions := 0
	for _, definition := range definitions {
		for _, action := range definition.Operations {
			workflowActions++
			parameters, err := tools.JSONSchemaToToolParameters(action.InputSchema)
			if err != nil {
				t.Fatalf("%s/%s schema projection failed: %v", definition.ID, action.ID, err)
			}
			byName := make(map[string]tools.ToolParameter, len(parameters))
			for _, parameter := range parameters {
				byName[parameter.Name] = parameter
				if strings.TrimSpace(parameter.Label.Get("zh_Hans")) == "" ||
					strings.TrimSpace(parameter.HumanDescription.Get("zh_Hans")) == "" ||
					strings.TrimSpace(parameter.Placeholder.Get("zh_Hans")) == "" {
					t.Errorf("%s/%s parameter %s lacks a Chinese form contract: %#v", definition.ID, action.ID, parameter.Name, parameter)
				}
			}

			required := schemaStringSet(action.InputSchema["required"])
			properties, _ := action.InputSchema["properties"].(map[string]interface{})
			for name, raw := range properties {
				property, _ := raw.(map[string]interface{})
				parameter, exists := byName[name]
				if !exists {
					t.Errorf("%s/%s property %s is missing from Workflow form", definition.ID, action.ID, name)
					continue
				}
				if required[name] != parameter.Required {
					t.Errorf("%s/%s property %s required=%v, projected=%v", definition.ID, action.ID, name, required[name], parameter.Required)
				}
				if property["type"] == "string" {
					switch property["format"] {
					case "date":
						if parameter.Type != tools.ToolParameterTypeDate {
							t.Errorf("%s/%s property %s should use date editor, got %s", definition.ID, action.ID, name, parameter.Type)
						}
					case "date-time":
						if parameter.Type != tools.ToolParameterTypeDateTime {
							t.Errorf("%s/%s property %s should use date-time editor, got %s", definition.ID, action.ID, name, parameter.Type)
						}
					}
				}
				if property["type"] == "array" {
					items, _ := property["items"].(map[string]interface{})
					if items["type"] == "string" && parameter.Type != tools.ToolParameterTypeStringList {
						t.Errorf("%s/%s property %s should use string-list editor, got %s", definition.ID, action.ID, name, parameter.Type)
					}
				}
			}
		}
	}
	if workflowActions == 0 {
		t.Fatal("connector catalog exposes no Workflow actions")
	}
}

func supportsWorkflow(callers []tools.ToolInvokeFrom) bool {
	for _, caller := range callers {
		if caller == tools.ToolInvokeFromWorkflow {
			return true
		}
	}
	return false
}

func schemaStringSet(raw interface{}) map[string]bool {
	out := map[string]bool{}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			out[value] = true
		}
	case []interface{}:
		for _, rawValue := range values {
			if value, ok := rawValue.(string); ok {
				out[value] = true
			}
		}
	}
	return out
}
