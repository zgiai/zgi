package llm

import "testing"

func TestNodeIsContentSupportedByModel_GLMSupportsImageInput(t *testing.T) {
	n := &Node{}

	providers := []string{"glm", "zhipu", "bigmodel"}
	for _, provider := range providers {
		if !n.isContentSupportedByModel(
			PromptMessageContent{Type: PromptMessageContentTypeImage},
			&ModelConfigWithCredentialsEntity{Provider: provider},
		) {
			t.Fatalf("provider %q should support image input", provider)
		}

		if n.isContentSupportedByModel(
			PromptMessageContent{Type: PromptMessageContentTypeAudio},
			&ModelConfigWithCredentialsEntity{Provider: provider},
		) {
			t.Fatalf("provider %q should not support audio input by default", provider)
		}
	}
}
