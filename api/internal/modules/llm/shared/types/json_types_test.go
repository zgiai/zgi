package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringArray_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
		wantErr  bool
	}{
		{
			name:     "PostgreSQL array format",
			input:    []byte("{text-chat,vision,function-calling}"),
			expected: []string{"text-chat", "vision", "function-calling"},
			wantErr:  false,
		},
		{
			name:     "PostgreSQL empty array",
			input:    []byte("{}"),
			expected: []string{},
			wantErr:  false,
		},
		{
			name:     "PostgreSQL array with quoted strings",
			input:    []byte(`{"text-chat","vision with spaces","function-calling"}`),
			expected: []string{"text-chat", "vision with spaces", "function-calling"},
			wantErr:  false,
		},
		{
			name:     "JSON array format",
			input:    []byte(`["text-chat","vision","function-calling"]`),
			expected: []string{"text-chat", "vision", "function-calling"},
			wantErr:  false,
		},
		{
			name:     "JSON empty array",
			input:    []byte(`[]`),
			expected: []string{},
			wantErr:  false,
		},
		{
			name:     "Null value",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "String type input (PostgreSQL)",
			input:    "{embedding,rerank}",
			expected: []string{"embedding", "rerank"},
			wantErr:  false,
		},
		{
			name:     "String type input (JSON)",
			input:    `["embedding","rerank"]`,
			expected: []string{"embedding", "rerank"},
			wantErr:  false,
		},
		{
			name:     "Single element PostgreSQL array",
			input:    []byte("{text-chat}"),
			expected: []string{"text-chat"},
			wantErr:  false,
		},
		{
			name:     "Single element JSON array",
			input:    []byte(`["text-chat"]`),
			expected: []string{"text-chat"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s StringArray
			err := s.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, []string(s))
			}
		})
	}
}

func TestStringArray_Value(t *testing.T) {
	tests := []struct {
		name     string
		input    StringArray
		expected string
		wantErr  bool
	}{
		{
			name:     "Normal array",
			input:    StringArray{"text-chat", "vision"},
			expected: `{text-chat,vision}`,
			wantErr:  false,
		},
		{
			name:     "Empty array",
			input:    StringArray{},
			expected: `{}`,
			wantErr:  false,
		},
		{
			name:     "Nil array",
			input:    nil,
			expected: `{}`,
			wantErr:  false,
		},
		{
			name:     "Single element",
			input:    StringArray{"embedding"},
			expected: `{embedding}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.input.Value()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				var actual string
				switch typed := val.(type) {
				case string:
					actual = typed
				case []byte:
					actual = string(typed)
				default:
					t.Fatalf("unexpected driver.Value type %T", val)
				}
				assert.Equal(t, tt.expected, actual)
			}
		})
	}
}

func TestSplitPostgresArray(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple values",
			input:    "text-chat,vision,function-calling",
			expected: []string{"text-chat", "vision", "function-calling"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Single value",
			input:    "text-chat",
			expected: []string{"text-chat"},
		},
		{
			name:     "Quoted values with comma",
			input:    `"text-chat","value,with,comma","normal"`,
			expected: []string{"text-chat", "value,with,comma", "normal"},
		},
		{
			name:     "Mixed quoted and unquoted",
			input:    `text-chat,"vision with spaces",embedding`,
			expected: []string{"text-chat", "vision with spaces", "embedding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitPostgresArray(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
