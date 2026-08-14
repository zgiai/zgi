package service

import "testing"

func TestConversationTitleLooksUnprocessed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		title      string
		firstQuery string
		want       bool
	}{
		{name: "empty", title: "", firstQuery: "explain access approval", want: true},
		{name: "default", title: defaultConversationTitle, firstQuery: "explain access approval", want: true},
		{name: "english timestamp", title: "Conversation 2026-08-14 17:12:28", firstQuery: "explain access approval", want: true},
		{name: "chinese timestamp", title: "\u4f1a\u8bdd 2026-08-14 17:12:28", firstQuery: "explain access approval", want: true},
		{name: "first query fallback", title: "explain access approval", firstQuery: "explain access approval", want: true},
		{name: "generated title", title: "Production API access approvals", firstQuery: "explain access approval", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := conversationTitleLooksUnprocessed(tt.title, tt.firstQuery); got != tt.want {
				t.Fatalf("conversationTitleLooksUnprocessed(%q, %q) = %v, want %v", tt.title, tt.firstQuery, got, tt.want)
			}
		})
	}
}

func TestConversationMetadataWithTitleStatusPreservesMetadata(t *testing.T) {
	t.Parallel()

	original := map[string]interface{}{"surface": aiChatSurfaceWorkChat}
	got := conversationMetadataWithTitleStatus(original, conversationTitleStatusPending)
	if got["surface"] != aiChatSurfaceWorkChat {
		t.Fatalf("surface = %#v, want %q", got["surface"], aiChatSurfaceWorkChat)
	}
	if got[conversationTitleGenerationStatusKey] != conversationTitleStatusPending {
		t.Fatalf("title status = %#v, want %q", got[conversationTitleGenerationStatusKey], conversationTitleStatusPending)
	}
	if _, exists := original[conversationTitleGenerationStatusKey]; exists {
		t.Fatal("conversationMetadataWithTitleStatus mutated the original metadata")
	}
}
