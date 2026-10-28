package agentmemory

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRuntimeSlotReadsAutoWriteFromPublishedSnapshots(t *testing.T) {
	var slots []RuntimeSlot
	raw := `[{"key":"a","max_chars":100,"enabled":true},{"key":"b","enabled":true,"auto_write":false},{"key":"c","enabled":true,"auto_write":true,"read_only":true},{"key":"d","enabled":true,"manual_only":true}]`
	if err := json.Unmarshal([]byte(raw), &slots); err != nil {
		t.Fatal(err)
	}
	if slots[0].ManualOnly || !slots[1].ManualOnly || slots[2].ManualOnly || !slots[2].ReadOnly || !slots[3].ManualOnly || slots[0].MaxChars != 100 {
		t.Fatalf("slots = %+v", slots)
	}
}

func TestSlotWriteRules(t *testing.T) {
	for name, tc := range map[string]struct {
		slot   RuntimeSlot
		mode   string
		reject bool
	}{
		"plain explicit":         {RuntimeSlot{Key: "a"}, MutationModeExplicit, false},
		"plain proactive":        {RuntimeSlot{Key: "a"}, MutationModeProactive, false},
		"read-only explicit":     {RuntimeSlot{Key: "a", ReadOnly: true}, MutationModeExplicit, true},
		"manual-only explicit":   {RuntimeSlot{Key: "a", ManualOnly: true}, MutationModeExplicit, false},
		"manual-only proactive":  {RuntimeSlot{Key: "a", ManualOnly: true}, MutationModeProactive, true},
		"manual-only mixed case": {RuntimeSlot{Key: "a", ManualOnly: true}, " Proactive ", true},
	} {
		err := CheckSlotWriteRules(tc.slot, tc.mode)
		if (err != nil) != tc.reject || (err != nil && !errors.Is(err, ErrUnauthorized)) {
			t.Fatalf("%s: err = %v, want reject %v", name, err, tc.reject)
		}
	}
}
