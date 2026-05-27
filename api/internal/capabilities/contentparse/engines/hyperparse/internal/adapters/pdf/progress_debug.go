package pdf

import (
	"log"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"
)

// CONTENT_PARSE_FULLDOC_LOG enables adapter-level progress logs when set to a
// truthy value. DOCSTILL_FULLDOC_LOG remains a legacy alias.

func fullDocDebugEnabled() bool {
	raw := envconfig.String("CONTENT_PARSE_FULLDOC_LOG")
	if raw == "" {
		raw = envconfig.String("DOCSTILL_FULLDOC_LOG")
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
