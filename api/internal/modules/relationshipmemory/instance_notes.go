package relationshipmemory

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxInstanceNotes = 200

// DigitalHumanMemoryFilesEnabled selects the file view of DigitalHuman memory
// (ADR 0014). It is fail-closed.
func DigitalHumanMemoryFilesEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ZGI_DIGITAL_HUMAN_MEMORY_FILES_ENABLED")))
	return err == nil && enabled
}

// InstanceNote is one of a DigitalHuman's notes on a person (ADR 0006
// instance memory), shown to that person read-only under /user/notes/.
type InstanceNote struct {
	Path      string    `json:"path"`
	Tags      string    `json:"tags,omitempty"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InstanceNotes returns the active notes a DigitalHuman keeps on one person,
// newest first. The DigitalHuman owns them; the person may see them but not
// change them.
func (s *Service) InstanceNotes(ctx context.Context, workspaceID, digitalHumanID, subjectID uuid.UUID) ([]InstanceNote, error) {
	if workspaceID == uuid.Nil || digitalHumanID == uuid.Nil || subjectID == uuid.Nil {
		return []InstanceNote{}, nil
	}
	var rows []struct {
		MemoryKey  string
		MemoryType string
		Content    string
		UpdatedAt  time.Time
	}
	err := s.db.WithContext(ctx).Table("digital_human_memories AS m").
		Select("m.memory_key, m.memory_type, m.content, m.updated_at").
		Joins("JOIN digital_human_end_user_instances AS i ON m.scope_id = i.id::text AND i.digital_human_id = m.digital_human_id").
		Where("i.workspace_id = ? AND i.digital_human_id = ? AND i.subject_id = ? AND m.memory_class = 'business' AND m.scope_type = 'instance' AND m.status = 'active'",
			workspaceID, digitalHumanID, subjectID).
		Order("m.updated_at DESC").Limit(maxInstanceNotes).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list instance notes: %w", err)
	}
	notes := make([]InstanceNote, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, InstanceNote{
			Path: "/user/notes/" + strings.ReplaceAll(strings.TrimSpace(row.MemoryKey), "/", "_") + ".md",
			Tags: row.MemoryType, Content: row.Content, UpdatedAt: row.UpdatedAt,
		})
	}
	return notes, nil
}
