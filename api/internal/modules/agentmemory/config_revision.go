package agentmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

const (
	maxRevisionSlotChars        = 5000
	maxRevisionDescriptionChars = 1000
)

type configRevisionSlot struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
	// The write rules are omitted when off, so revisions of configurations
	// that predate them do not change.
	ReadOnly   bool `json:"read_only,omitempty"`
	ManualOnly bool `json:"manual_only,omitempty"`
}

// ConfigRevision returns the canonical revision fence for Agent memory
// configuration. Keeping this in the memory domain lets legacy published
// snapshots and current runtime responses derive the same fallback revision.
func ConfigRevision(enabled, automatic bool, slots []RuntimeSlot) string {
	payload := struct {
		Enabled   bool                 `json:"enabled"`
		Automatic bool                 `json:"automatic"`
		Slots     []configRevisionSlot `json:"slots"`
	}{
		Enabled:   enabled,
		Automatic: automatic,
		Slots:     normalizeConfigRevisionSlots(slots),
	}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func normalizeConfigRevisionSlots(slots []RuntimeSlot) []configRevisionSlot {
	if len(slots) == 0 {
		return []configRevisionSlot{}
	}

	normalized := make([]configRevisionSlot, 0, len(slots))
	seen := make(map[string]struct{}, len(slots))
	for index, slot := range slots {
		key := strings.ToLower(strings.TrimSpace(slot.Key))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		// A missing limit means the 2,000-character default, as it always has.
		maxChars := defaultSlotMaxChars
		if slot.MaxChars > 0 && slot.MaxChars <= maxRevisionSlotChars {
			maxChars = slot.MaxChars
		}
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = index
		}
		normalized = append(normalized, configRevisionSlot{
			Key:         key,
			Name:        truncateConfigRevisionRunes(strings.TrimSpace(slot.Name), 80),
			Description: truncateConfigRevisionRunes(strings.TrimSpace(slot.Description), maxRevisionDescriptionChars),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
			ReadOnly:    slot.ReadOnly,
			ManualOnly:  slot.ManualOnly,
		})
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder != normalized[j].SortOrder {
			return normalized[i].SortOrder < normalized[j].SortOrder
		}
		return normalized[i].Key < normalized[j].Key
	})
	return normalized
}

func truncateConfigRevisionRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
