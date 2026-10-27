package relationshipmemory

import "time"

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
