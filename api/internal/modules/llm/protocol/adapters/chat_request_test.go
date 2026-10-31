package adapter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestChatRequestJSONDoesNotExposeProviderHint(t *testing.T) {
	request := ChatRequest{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Messages: []Message{
			{Role: "user", Content: "hello"},
		},
	}

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	body := string(payload)
	if strings.Contains(body, `"provider"`) {
		t.Fatalf("json payload = %s, want provider hint omitted", body)
	}
	if !strings.Contains(body, `"model":"deepseek-chat"`) {
		t.Fatalf("json payload = %s, want model preserved", body)
	}
	if !strings.Contains(body, `"messages"`) {
		t.Fatalf("json payload = %s, want messages preserved", body)
	}
}

func TestNormalizeSystemMessagesMovesAndMergesTextBeforeConversation(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "base instructions"},
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "working", ToolCalls: []ToolCall{{ID: "call-1"}}},
		{Role: "tool", ToolCallID: "call-1", Content: "result"},
		{Role: "SYSTEM", Content: []MessageContentPart{{Type: "text", Text: "latest runtime guidance"}}},
	}
	original := append([]Message(nil), messages...)

	normalized := NormalizeSystemMessages(messages)

	if len(normalized) != 4 {
		t.Fatalf("len(normalized) = %d, want 4: %#v", len(normalized), normalized)
	}
	if normalized[0].Role != "system" {
		t.Fatalf("normalized[0].Role = %q, want system", normalized[0].Role)
	}
	if got, want := normalized[0].Content, "base instructions\n\nlatest runtime guidance"; got != want {
		t.Fatalf("normalized system content = %#v, want %#v", got, want)
	}
	for index, wantRole := range []string{"user", "assistant", "tool"} {
		if got := normalized[index+1].Role; got != wantRole {
			t.Fatalf("normalized[%d].Role = %q, want %q", index+1, got, wantRole)
		}
	}
	if !reflect.DeepEqual(messages, original) {
		t.Fatalf("NormalizeSystemMessages() mutated input:\nactual: %#v\nwant: %#v", messages, original)
	}
}

func TestNormalizeSystemMessagesKeepsStructuredSystemContentLosslessly(t *testing.T) {
	structured := map[string]interface{}{"type": "provider_specific", "value": "keep me"}
	messages := []Message{
		{Role: "user", Content: "question"},
		{Role: "system", Content: "text instructions"},
		{Role: "SYSTEM", Content: structured},
		{Role: "assistant", Content: "answer"},
	}

	normalized := NormalizeSystemMessages(messages)

	if len(normalized) != len(messages) {
		t.Fatalf("len(normalized) = %d, want %d", len(normalized), len(messages))
	}
	if normalized[0].Role != "system" || normalized[0].Content != "text instructions" {
		t.Fatalf("normalized[0] = %#v, want first system message", normalized[0])
	}
	if normalized[1].Role != "system" || !reflect.DeepEqual(normalized[1].Content, structured) {
		t.Fatalf("normalized[1] = %#v, want structured system message", normalized[1])
	}
	if normalized[2].Role != "user" || normalized[3].Role != "assistant" {
		t.Fatalf("conversation order = %#v, want user then assistant", normalized[2:])
	}
}

func TestPlaceSystemMessagesKeepsLaterGuidanceInPlace(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "base instructions"},
		{Role: "system", Content: "more base instructions"},
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "working", ToolCalls: []ToolCall{{ID: "call-1"}}},
		{Role: "tool", ToolCallID: "call-1", Content: "result"},
		{Role: "SYSTEM", Content: []MessageContentPart{{Type: "text", Text: "latest runtime guidance"}}},
	}
	original := append([]Message(nil), messages...)

	normalized := PlaceSystemMessages(messages)

	roles := []string{}
	for _, message := range normalized {
		roles = append(roles, message.Role)
	}
	if strings.Join(roles, ",") != "system,user,assistant,tool,user" {
		t.Fatalf("roles = %v", roles)
	}
	if got, want := normalized[0].Content, "base instructions"; got != want {
		t.Fatalf("system content = %#v, want only the first system message %#v", got, want)
	}
	if got, want := normalized[1].Content, "<system-reminder>\nmore base instructions\n</system-reminder>\n\nquestion"; got != want {
		t.Fatalf("second system message = %#v, want a reminder before the question %#v", got, want)
	}
	if got, want := normalized[4].Content, "<system-reminder>\nlatest runtime guidance\n</system-reminder>"; got != want {
		t.Fatalf("later guidance = %#v, want a reminder in place %#v", got, want)
	}
	if !reflect.DeepEqual(messages, original) {
		t.Fatalf("PlaceSystemMessages() mutated input:\nactual: %#v\nwant: %#v", messages, original)
	}
}

// The prefix up to the previous turn stays the same when the next turn adds
// its changing guidance, which is what lets providers cache it (ADR 0032).
func TestPlaceSystemMessagesKeepsThePrefixStableAcrossTurns(t *testing.T) {
	turn := func(now string, history ...Message) []Message {
		messages := append([]Message{{Role: "system", Content: "persona"}}, history...)
		return PlaceSystemMessages(append(messages,
			Message{Role: "system", Content: "Current time: " + now},
			Message{Role: "user", Content: "memory notes"},
			Message{Role: "system", Content: "turn boundary"},
			Message{Role: "user", Content: "the request"},
			Message{Role: "system", Content: "skill catalog"},
		))
	}
	first := turn("15:00")
	history := []Message{{Role: "user", Content: "earlier"}, {Role: "assistant", Content: "answer"}}
	second := turn("15:05", history...)
	if second[0].Content != first[0].Content || !reflect.DeepEqual(second[1:3], history) {
		t.Fatalf("prefix changed: %#v", second[:3])
	}
	var current string
	for _, message := range second[3:] {
		if message.Role != "user" {
			t.Fatalf("current turn = %#v, want user messages only", second[3:])
		}
		current += message.Content.(string) + "\n"
	}
	for _, want := range []string{"Current time: 15:05", "memory notes", "turn boundary", "the request", "skill catalog"} {
		if !strings.Contains(current, want) {
			t.Fatalf("current turn %q lacks %q", current, want)
		}
	}
	if strings.Index(current, "the request") > strings.Index(current, "skill catalog") {
		t.Fatal("guidance order changed")
	}
}

func TestPlaceSystemMessagesMergesRemindersWithImageParts(t *testing.T) {
	image := []MessageContentPart{{Type: "text", Text: "look"}, {Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,aGVsbG8="}}}
	normalized := PlaceSystemMessages([]Message{
		{Role: "system", Content: "persona"},
		{Role: "system", Content: "turn context"},
		{Role: "user", Content: image},
		{Role: "system", Content: "skill catalog"},
	})
	if len(normalized) != 2 {
		t.Fatalf("normalized = %#v", normalized)
	}
	parts, ok := normalized[1].Content.([]MessageContentPart)
	if !ok || len(parts) != 4 || !strings.Contains(parts[0].Text, "turn context") || parts[2].ImageURL == nil || !strings.Contains(parts[3].Text, "skill catalog") {
		t.Fatalf("merged content = %#v", normalized[1].Content)
	}
}

// Only reminders merge: two original messages stay apart, and a merged
// message keeps the original's transcript identity for compaction.
func TestPlaceSystemMessagesKeepsOriginalMessagesAndEntryIDs(t *testing.T) {
	normalized := PlaceSystemMessages([]Message{
		{Role: "system", Content: "persona"},
		{Role: "user", Content: "summary", EntryID: "entry-summary"},
		{Role: "user", Content: "older request", EntryID: "entry-older"},
		{Role: "system", Content: "turn boundary"},
		{Role: "user", Content: "current request", EntryID: "entry-current"},
	})
	if len(normalized) != 4 {
		t.Fatalf("normalized = %#v, want the two original user messages kept apart", normalized)
	}
	if normalized[1].EntryID != "entry-summary" || normalized[2].EntryID != "entry-older" {
		t.Fatalf("original entries = %q, %q", normalized[1].EntryID, normalized[2].EntryID)
	}
	if normalized[2].Content != "older request\n\n<system-reminder>\nturn boundary\n</system-reminder>" {
		t.Fatalf("reminder joined %#v, want it after the older request", normalized[2].Content)
	}
	if normalized[3].EntryID != "entry-current" || normalized[3].Content != "current request" {
		t.Fatalf("current request = %#v", normalized[3])
	}
	leadingReminder := PlaceSystemMessages([]Message{
		{Role: "system", Content: "persona"},
		{Role: "system", Content: "turn context"},
		{Role: "user", Content: "request", EntryID: "entry-request"},
	})
	if len(leadingReminder) != 2 || leadingReminder[1].EntryID != "entry-request" {
		t.Fatalf("reminder before the request = %#v, want the request's entry kept", leadingReminder)
	}
}
