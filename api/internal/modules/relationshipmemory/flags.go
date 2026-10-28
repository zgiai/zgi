package relationshipmemory

import (
	"os"
	"strings"
)

// AutomaticExtractionEnabled is the deployment fuse for automatic memory. It
// is on unless an operator turns it off; the legacy variable is still read
// when the current one is unset, so a deployment that turned automatic
// memory off under the old name keeps it off.
func AutomaticExtractionEnabled() bool {
	value := strings.TrimSpace(os.Getenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED"))
	if value == "" {
		value = os.Getenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED")
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
