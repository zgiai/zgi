package adapter

import (
	"reflect"
	"testing"
)

// A turn that carries its reminders in its user message sends what placing
// them as system messages would send, so the first turn's request and cache
// do not change (ADR 0043).
func TestPrependSystemRemindersMatchesPlacement(t *testing.T) {
	for _, content := range []interface{}{
		"What time is it?",
		[]MessageContentPart{{Type: "text", Text: "Describe this."}, {Type: "image_url", ImageURL: &ImageURL{URL: "https://example.test/a.png"}}},
	} {
		placed := PlaceSystemMessages([]Message{
			{Role: "system", Content: "System prompt."},
			{Role: "system", Content: "Current time: now."},
			{Role: "system", Content: "Turn boundary."},
			{Role: "user", Content: content},
		})
		want := placed[1].Content
		got := PrependSystemReminders([]string{"Current time: now.", " ", "Turn boundary."}, content)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("PrependSystemReminders() = %#v, want %#v", got, want)
		}
	}
	if got := PrependSystemReminders(nil, "Hello"); got != "Hello" {
		t.Fatalf("no reminders = %#v, want the content unchanged", got)
	}
}
