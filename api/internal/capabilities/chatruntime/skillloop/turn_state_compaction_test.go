package skillloop

import (
	"strings"
	"testing"
)

func TestNormalizeTurnStateItemBoundsModelOnlyValue(t *testing.T) {
	item := normalizeTurnStateItem(map[string]interface{}{
		"kind":       "working_fact",
		"visibility": "model_only",
		"key":        "large_value",
		"value":      strings.Repeat("界", turnStateTextValueMaxRunes+200),
	})
	got := []rune(stringFromInterface(item["value"]))
	if len(got) != turnStateTextValueMaxRunes {
		t.Fatalf("value runes = %d, want %d", len(got), turnStateTextValueMaxRunes)
	}
}

func TestNormalizeTurnStateItemsRejectsOversizedUserDeliverableContent(t *testing.T) {
	_, err := normalizeTurnStateItems(map[string]interface{}{"items": []interface{}{map[string]interface{}{
		"kind":       "user_deliverable",
		"visibility": "user_visible",
		"content":    strings.Repeat("章", turnStateTextValueMaxRunes+200),
	}}})
	if err == nil || !strings.Contains(err.Error(), "{ref, digest, summary}") {
		t.Fatalf("normalizeTurnStateItems() error = %v, want compact reference guidance", err)
	}
}

func TestTurnStateItemReceiptsDoNotEchoValues(t *testing.T) {
	receipts := turnStateItemReceipts([]map[string]interface{}{{
		"kind":       "working_fact",
		"visibility": "model_only",
		"key":        "chapter_summary",
		"value":      "sensitive reusable content",
		"source":     "file-reader/read_file",
	}})
	if len(receipts) != 1 {
		t.Fatalf("receipts = %#v, want one", receipts)
	}
	if _, ok := receipts[0]["value"]; ok {
		t.Fatalf("receipt must not echo value: %#v", receipts[0])
	}
	if receipts[0]["recorded_chars"] != len([]rune("sensitive reusable content")) {
		t.Fatalf("recorded_chars = %#v", receipts[0]["recorded_chars"])
	}
}
