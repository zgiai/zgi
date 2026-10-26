package relationshipmemory

import (
	"os"
	"strings"
)

// AutomaticExtractionEnabled is fail-closed. The legacy variable is accepted
// for one release so a rolling deployment has one consistent extraction fence.
func AutomaticExtractionEnabled() bool {
	value := strings.TrimSpace(os.Getenv("ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED"))
	if value == "" {
		value = os.Getenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED")
	}
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
