package service

import (
	"os"
	"strings"
	"unicode/utf8"
)

func aichatTimelineDebugEnabled() bool {
	value := strings.TrimSpace(os.Getenv("AICHAT_TIMELINE_DEBUG"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func timelineDebugString(value interface{}) string {
	return strings.TrimSpace(stringFromAny(value))
}

func timelineDebugTextLen(value interface{}) int {
	text := stringFromAny(value)
	if text == "" {
		return 0
	}
	return utf8.RuneCountInString(text)
}
