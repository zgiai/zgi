package relationshipmemory

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// DigitalHumanMemoryFilesEnabled selects the file view of DigitalHuman memory
// (ADR 0014). It is fail-closed.
func DigitalHumanMemoryFilesEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ZGI_DIGITAL_HUMAN_MEMORY_FILES_ENABLED")))
	return err == nil && enabled
}

// InstanceNote is one of a DigitalHuman's notes on a person (ADR 0006
// instance memory), shown to that person read-only under /user/notes/.
type InstanceNote struct {
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Tags      string    `json:"tags,omitempty"`
	Content   string    `json:"content"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}
