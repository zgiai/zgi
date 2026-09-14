package skills

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// A package may describe script arguments, but cannot introduce providers or
// external schema fetches. Bound complexity before invoking the schema compiler.
func validatePackageScriptSchema(schema map[string]interface{}) error {
	raw, err := json.Marshal(schema)
	if err != nil || len(raw) > 128*1024 {
		return fmt.Errorf("script_input_schema must be JSON and at most 128 KiB")
	}
	if schema["type"] != "object" {
		return fmt.Errorf("script_input_schema root must have type object")
	}
	var walk func(interface{}, int) error
	walk = func(value interface{}, depth int) error {
		if depth > 32 {
			return fmt.Errorf("script_input_schema exceeds maximum depth")
		}
		switch node := value.(type) {
		case map[string]interface{}:
			for key, child := range node {
				if key == "$ref" || key == "$dynamicRef" {
					ref, ok := child.(string)
					if !ok || !strings.HasPrefix(ref, "#") {
						return fmt.Errorf("script_input_schema permits only local references")
					}
				}
				if key == "$id" {
					return fmt.Errorf("script_input_schema must not change its resolution scope")
				}
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		case []interface{}:
			for _, child := range node {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(schema, 0); err != nil {
		return err
	}
	if err := tools.ValidateJSONSchema(schema); err != nil {
		return fmt.Errorf("invalid script_input_schema: %w", err)
	}
	return nil
}
