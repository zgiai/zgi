package skillloop

import (
	"fmt"
	"strings"
)

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.Contains(rawURL, "download=") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func payloadMap(source map[string]interface{}, key string) map[string]interface{} {
	if source == nil {
		return nil
	}
	value, ok := source[key]
	if !ok || value == nil {
		return nil
	}
	if mapped, ok := value.(map[string]interface{}); ok {
		return mapped
	}
	return nil
}
