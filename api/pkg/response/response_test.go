package response

import "testing"

func TestGetHTTPStatusFor5DigitCode(t *testing.T) {
	tests := []struct {
		name         string
		code         int
		expectedHTTP int
	}{
		// Authentication errors (x01xx) -> 401
		{
			name:         "LLM Invalid API Key",
			code:         40101,
			expectedHTTP: 401,
		},
		{
			name:         "LLM API Key Disabled",
			code:         40102,
			expectedHTTP: 401,
		},

		// Authorization errors (x03xx) -> 403
		{
			name:         "LLM Insufficient Balance",
			code:         40301,
			expectedHTTP: 403,
		},
		{
			name:         "LLM Model Not Authorized",
			code:         40302,
			expectedHTTP: 403,
		},

		// Not Found errors (x04xx) -> 404
		{
			name:         "LLM Model Not Found",
			code:         40401,
			expectedHTTP: 404,
		},
		{
			name:         "LLM Route Not Found",
			code:         40404,
			expectedHTTP: 404,
		},

		// Upstream errors (x05xx) -> varies
		{
			name:         "LLM Upstream Auth Failed",
			code:         40501,
			expectedHTTP: 502,
		},
		{
			name:         "LLM Upstream Rate Limit",
			code:         40502,
			expectedHTTP: 429,
		},
		{
			name:         "LLM Upstream Timeout",
			code:         40503,
			expectedHTTP: 504,
		},
		{
			name:         "LLM Upstream Unavailable",
			code:         40504,
			expectedHTTP: 503,
		},
		{
			name:         "LLM No Provider Available",
			code:         40506,
			expectedHTTP: 503,
		},

		// System errors (x06xx) -> 500
		{
			name:         "LLM Internal Error",
			code:         40601,
			expectedHTTP: 500,
		},
		{
			name:         "LLM Database Error",
			code:         40602,
			expectedHTTP: 500,
		},

		// Rate limit (x09xx) -> 429
		{
			name:         "LLM Rate Limit Exceeded",
			code:         40901,
			expectedHTTP: 429,
		},

		// Parameter validation (x00xx) -> 400
		{
			name:         "LLM Missing Model",
			code:         40001,
			expectedHTTP: 400,
		},
		{
			name:         "LLM Invalid Messages",
			code:         40003,
			expectedHTTP: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getHTTPStatusFromErrorCode(tt.code)
			if got != tt.expectedHTTP {
				t.Errorf("getHTTPStatusFromErrorCode(%d) = %d, want %d",
					tt.code, got, tt.expectedHTTP)
			}
		})
	}
}

func TestValidateErrorCode(t *testing.T) {
	tests := []struct {
		name  string
		code  int
		valid bool
	}{
		{"Success code", 0, true},
		{"LLM 5-digit code", 40101, true},
		{"Account 5-digit code", 20101, true},
		{"Legacy 6-digit param error", 100001, true},
		{"Legacy 6-digit auth error", 400001, true},
		{"Invalid too small", 5000, false},
		{"Invalid 4-digit", 4000, false},
		{"Invalid 7-digit", 1000000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateErrorCode(tt.code)
			if got != tt.valid {
				t.Errorf("ValidateErrorCode(%d) = %v, want %v",
					tt.code, got, tt.valid)
			}
		})
	}
}

func TestGetErrorCodeRange(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{0, "SUCCESS"},
		{100001, "PARAM_ERROR"},
		{200001, "BUSINESS_ERROR"},
		{300001, "SYSTEM_ERROR"},
		{400001, "AUTH_ERROR"},
		{500001, "THIRD_PARTY_ERROR"},
		{40101, "UNKNOWN"}, // 5-digit codes return UNKNOWN from GetErrorCodeRange
		{99999, "UNKNOWN"},
	}

	for _, tt := range tests {
		got := GetErrorCodeRange(tt.code)
		if got != tt.expected {
			t.Errorf("GetErrorCodeRange(%d) = %s, want %s",
				tt.code, got, tt.expected)
		}
	}
}
