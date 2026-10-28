package agentmemory

// The vocabulary Agent memory shares with the relationship store and the chat
// runtime: scopes, sources, the runtime view of a memory file, and the
// metadata every change carries. The values themselves live in the
// relationship store (ADR 0016, ADR 0017).

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	UserScopeAccount = "account"
	UserScopeEndUser = "end_user"

	SourceKindLegacy    = "legacy"
	SourceKindExplicit  = "explicit"
	SourceKindAutomatic = "automatic"
	SourceKindManager   = "manager"

	EventActorOrganizer = "organizer"
	EventActorUser      = "user"
	EventActorModel     = "model"

	EventSourceAPI   = "api"
	EventSourceAgent = "agent"
)

const (
	ConfigScopeDraft     = "draft"
	ConfigScopePublished = "published"
)

// MaxSlotChars is the most a memory value held as one runtime slot may contain.
const MaxSlotChars = 5000

var (
	ErrInvalidInput = errors.New("invalid agent memory input")
	ErrNotFound     = errors.New("agent memory not found")
	ErrUnauthorized = errors.New("agent memory requester is unauthorized")
	ErrConflict     = errors.New("agent memory revision conflict")
)

type RuntimeSlot struct {
	Key         string `json:"key"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
	// ReadOnly blocks are never written by the model.
	ReadOnly bool `json:"read_only,omitempty"`
	// ManualOnly blocks take explicit writes only. It is the negation of a
	// block's auto_write, so snapshots that predate it allow automatic writes.
	ManualOnly bool `json:"manual_only,omitempty"`
}

// UnmarshalJSON also reads auto_write, the form published Agent snapshots
// use, so a block saved as auto_write: false stays manual-only.
func (s *RuntimeSlot) UnmarshalJSON(data []byte) error {
	type plain RuntimeSlot
	var decoded struct {
		plain
		AutoWrite *bool `json:"auto_write"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = RuntimeSlot(decoded.plain)
	if decoded.AutoWrite != nil && !*decoded.AutoWrite {
		s.ManualOnly = true
	}
	return nil
}

// checkSlotWriteRules rejects a model or background write the block does not
// allow: any write to a read-only block, and an automatic write to a
// manual-only one.
func checkSlotWriteRules(slot RuntimeSlot, mode string) error {
	if slot.ReadOnly {
		return fmt.Errorf("%w: memory block %s is read-only", ErrUnauthorized, slot.Key)
	}
	if slot.ManualOnly && mode == MutationModeProactive {
		return fmt.Errorf("%w: memory block %s takes explicit requests only", ErrUnauthorized, slot.Key)
	}
	return nil
}

// CheckSlotWriteRules is checkSlotWriteRules for callers outside the package
// that validate edits before submitting them.
func CheckSlotWriteRules(slot RuntimeSlot, mode string) error {
	return checkSlotWriteRules(slot, strings.ToLower(strings.TrimSpace(mode)))
}

type MutationMetadata struct {
	ActorType            string
	Source               string
	SourceConversationID *uuid.UUID
	SourceMessageID      *uuid.UUID
	SourceCompletedAt    *time.Time
	SourceKind           string
	ExtractorVersion     string
	OperationID          *uuid.UUID
	ExpectedRevision     *int64
	MemoryEpoch          *int64
	ConfigScope          string
	ConfigRevision       string
	EventResult          string
}
