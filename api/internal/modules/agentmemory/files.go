package agentmemory

import (
	"sort"
	"strings"
)

// Agent memory is a set of files the model names and organizes itself,
// guided by the builder's memory instructions (ADR 0017). Nothing about the
// files is configured; one user's files together are bounded by
// MaxMemoryChars.
const MaxMemoryChars = 10000

// ValidMemoryKey reports whether key may name a memory file: lowercase
// letters, digits, and underscores, starting with a letter.
func ValidMemoryKey(key string) bool {
	return memoryKeyPattern.MatchString(key)
}

// FileSlots describes the user's current memory files as runtime slots, in
// name order, so code that walks slots walks the files.
func FileSlots(values []SlotValueResponse) []RuntimeSlot {
	slots := make([]RuntimeSlot, 0, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value.Key)
		if key == "" || strings.TrimSpace(value.Content) == "" {
			continue
		}
		slots = append(slots, RuntimeSlot{Key: key, Name: key, MaxChars: MaxMemoryChars, Enabled: true})
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Key < slots[j].Key })
	for index := range slots {
		slots[index].SortOrder = index
	}
	return slots
}
