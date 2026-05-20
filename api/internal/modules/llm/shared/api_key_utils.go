package shared

import "strings"

// MaskAPIKey generates a masked version of an API key for display purposes
// Example: "sk-proj-abc123def456" -> "sk-proj-***456"
func MaskAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}

	// For very short keys, just show asterisks
	if len(apiKey) <= 8 {
		return "***"
	}

	// Find prefix (e.g., "sk-", "sk-proj-", etc.)
	var prefix string
	if strings.HasPrefix(apiKey, "sk-proj-") {
		prefix = "sk-proj-"
	} else if strings.HasPrefix(apiKey, "sk-") {
		prefix = "sk-"
	} else if strings.Contains(apiKey, "-") {
		// Generic prefix detection
		parts := strings.SplitN(apiKey, "-", 2)
		if len(parts) > 0 {
			prefix = parts[0] + "-"
		}
	}

	// Show last 4 characters
	suffix := apiKey[len(apiKey)-4:]

	return prefix + "***" + suffix
}
