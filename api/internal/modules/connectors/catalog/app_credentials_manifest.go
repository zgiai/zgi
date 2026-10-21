package catalog

import "encoding/json"

// Reject even null/empty new fields under v1; decoding into the expanded Go
// struct must not silently widen the old manifest grammar.
func hasNewCredentialMetadata(raw json.RawMessage) bool {
	var metadata map[string]json.RawMessage
	if json.Unmarshal(raw, &metadata) != nil {
		return true
	}
	for _, key := range []string{"credential_fields", "setup"} {
		if _, ok := metadata[key]; ok {
			return true
		}
	}
	var operations []map[string]json.RawMessage
	if json.Unmarshal(metadata["operations"], &operations) != nil {
		return true
	}
	for _, op := range operations {
		for _, key := range []string{"credential_subject", "required_scopes"} {
			if _, ok := op[key]; ok {
				return true
			}
		}
	}
	return false
}
