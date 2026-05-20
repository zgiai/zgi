package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONArray represents a PostgreSQL JSONB string array
type JSONArray []string

// Scan implements the sql.Scanner interface
func (j *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan JSONArray: expected []byte or string, got %T", value)
	}
	return json.Unmarshal(bytes, j)
}

// Value implements the driver.Valuer interface
func (j JSONArray) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	return json.Marshal(j)
}

// JSONObject represents a PostgreSQL JSONB object
type JSONObject map[string]interface{}

// Scan implements the sql.Scanner interface
func (j *JSONObject) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan JSONObject: expected []byte or string, got %T", value)
	}
	return json.Unmarshal(bytes, j)
}

// Value implements the driver.Valuer interface
func (j JSONObject) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return json.Marshal(j)
}

// JSONMap is an alias for JSONObject for backward compatibility
type JSONMap = JSONObject

// JSONStringArray is a custom type for JSONB string array (alias for JSONArray)
type JSONStringArray = JSONArray

// StringArray is a custom type for handling both JSON arrays and PostgreSQL TEXT[] arrays
type StringArray []string

// Scan implements the sql.Scanner interface
// Handles both JSON format ["a","b"] and PostgreSQL TEXT[] format {a,b}
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		// Try string type
		if str, ok := value.(string); ok {
			bytes = []byte(str)
		} else {
			return nil
		}
	}

	// Check if it's PostgreSQL array format (starts with '{')
	// PostgreSQL returns arrays as: {val1,val2,val3}
	if len(bytes) > 0 && bytes[0] == '{' {
		// Parse PostgreSQL array format
		str := string(bytes)
		// Remove { and }
		str = str[1 : len(str)-1]

		// Handle empty array
		if str == "" {
			*s = []string{}
			return nil
		}

		// Simple split by comma (works for most cases)
		// Note: This doesn't handle quoted strings with commas, but use_cases don't have that
		parts := []string{}
		for _, part := range splitPostgresArray(str) {
			parts = append(parts, part)
		}
		*s = parts
		return nil
	}

	// Otherwise, try JSON format
	return json.Unmarshal(bytes, s)
}

// Value implements the driver.Valuer interface
// Returns PostgreSQL array format for TEXT[] columns: {val1,val2,val3}
func (s StringArray) Value() (driver.Value, error) {
	if s == nil || len(s) == 0 {
		return "{}", nil
	}

	// Build PostgreSQL array format: {val1,val2,val3}
	// Note: Values with special characters should be quoted
	result := "{"
	for i, val := range s {
		if i > 0 {
			result += ","
		}
		// Quote values that contain special characters
		if needsQuoting(val) {
			result += `"` + escapePostgresString(val) + `"`
		} else {
			result += val
		}
	}
	result += "}"
	return result, nil
}

// needsQuoting checks if a string needs quoting in PostgreSQL array
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, ch := range s {
		if ch == ',' || ch == '{' || ch == '}' || ch == '"' || ch == '\\' || ch == ' ' {
			return true
		}
	}
	return false
}

// escapePostgresString escapes special characters for PostgreSQL strings
func escapePostgresString(s string) string {
	result := ""
	for _, ch := range s {
		if ch == '"' || ch == '\\' {
			result += "\\"
		}
		result += string(ch)
	}
	return result
}

// splitPostgresArray splits a PostgreSQL array string by comma
func splitPostgresArray(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := []string{}
	current := ""
	inQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuote = !inQuote
		} else if ch == ',' && !inQuote {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}
