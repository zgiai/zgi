package llm

import "testing"

func TestNodeIsContentSupportedByModel_MiniMaxDoesNotAssumeImageInput(t *testing.T) {
	n := &Node{}

	providers := []string{"minimax", "minmax"}
	for _, provider := range providers {
		if n.isContentSupportedByModel(
			PromptMessageContent{Type: PromptMessageContentTypeImage},
			&ModelConfigWithCredentialsEntity{Provider: provider},
		) {
			t.Fatalf("provider %q should not assume image input support for chat", provider)
		}

		if n.isContentSupportedByModel(
			PromptMessageContent{Type: PromptMessageContentTypeAudio},
			&ModelConfigWithCredentialsEntity{Provider: provider},
		) {
			t.Fatalf("provider %q should not support audio input by default", provider)
		}
	}
}
