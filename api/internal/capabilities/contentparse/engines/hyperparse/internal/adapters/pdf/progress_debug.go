package pdf

import (
	"log"
	"os"
	"strings"
)

// CONTENT_PARSE_FULLDOC_LOG enables adapter-level progress logs when set to a
// truthy value. DOCSTILL_FULLDOC_LOG remains a legacy alias.

func fullDocDebugEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("CONTENT_PARSE_FULLDOC_LOG"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("DOCSTILL_FULLDOC_LOG"))
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

func fullDocDebugf(format string, args ...any) {
	if !fullDocDebugEnabled() {
		return
	}
	log.Printf("[pdf_adapter] "+format, args...)
}
