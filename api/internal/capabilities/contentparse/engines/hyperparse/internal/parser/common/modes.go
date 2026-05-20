package common

import (
	"fmt"
	"strings"
)

func NormalizeValidationMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "strict":
		return "strict", nil
	case "relaxed":
		return "relaxed", nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", mode)
	}
}

func NormalizeExtractMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "meta":
		return "meta", nil
	case "text":
		return "text", nil
	case "chunk":
		return "chunk", nil
	case "resources":
		return "resources", nil
	case "images":
		return "images", nil
	case "fonts":
		return "fonts", nil
	case "full":
		return "full", nil
	default:
		return "", fmt.Errorf("unsupported extract mode: %s", mode)
	}
}
