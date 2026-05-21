package announcement

import (
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	if len(token) != tokenLength {
		t.Fatalf("generateToken() length = %d, want %d", len(token), tokenLength)
	}
	for _, char := range token {
		if !isTokenAlphabetChar(char) {
			t.Fatalf("generateToken() contains unsupported character %q", char)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  NodeConfig
		wantErr bool
	}{
		{
			name: "accepts default timeout",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
			},
		},
		{
			name: "accepts one week",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 7,
					Unit:     "day",
				},
			},
		},
		{
			name: "requires content",
			config: NodeConfig{
				Title: "Release notice",
				Timeout: TimeoutConfig{
					Duration: 1,
					Unit:     "day",
				},
			},
			wantErr: true,
		},
		{
			name: "requires title",
			config: NodeConfig{
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 1,
					Unit:     "day",
				},
			},
			wantErr: true,
		},
		{
			name: "rejects over one week",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 8,
					Unit:     "day",
				},
			},
			wantErr: true,
		},
		{
			name: "rejects title over max length",
			config: NodeConfig{
				Title:   strings.Repeat("a", MaxTitleLength+1),
				Content: "Release window starts at 10:00.",
			},
			wantErr: true,
		},
		{
			name: "rejects unsupported unit",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 1,
					Unit:     "week",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func isTokenAlphabetChar(char rune) bool {
	for _, allowed := range tokenAlphabet {
		if char == allowed {
			return true
		}
	}
	return false
}
