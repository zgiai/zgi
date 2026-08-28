package tools

import (
	"encoding/json"
	"fmt"
)

// maxNormalizedJSONValueBytes bounds the adapter-to-runtime normalization
// buffer. Integration actions already expose much smaller, schema-bounded
// results to models; this limit is a final guard against an accidental
// unbounded native Go value before schema validation runs.
const maxNormalizedJSONValueBytes = 4 * 1024 * 1024

// NormalizeJSONValue converts JSON-serializable native Go values (including
// typed slices such as []string, []int, and []map[string]interface{}) into the
// canonical map[string]interface{} / []interface{} representation expected by
// the JSON Schema validator and by persisted operation receipts.
//
// The JSON round trip is intentional: an Adapter boundary is a JSON contract,
// not a Go-type contract. Unsupported values, non-finite numbers, and overly
// large results fail closed before they can be exposed or persisted.
func NormalizeJSONValue(value interface{}) (interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize JSON value: %w", err)
	}
	if len(encoded) > maxNormalizedJSONValueBytes {
		return nil, fmt.Errorf("normalize JSON value: encoded value exceeds %d bytes", maxNormalizedJSONValueBytes)
	}

	var normalized interface{}
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, fmt.Errorf("normalize JSON value: %w", err)
	}
	return normalized, nil
}
