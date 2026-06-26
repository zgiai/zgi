package service

import (
	"strings"
	"testing"
)

func TestInitialConversationTitleDoesNotExposeQuery(t *testing.T) {
	query := `你帮我看看我的代码也没有什么问题# MODELMETA_API_URL=https://models`

	title := initialConversationTitle()

	if title != defaultConversationTitle {
		t.Fatalf("initialConversationTitle() = %q, want %q", title, defaultConversationTitle)
	}
	if strings.Contains(title, query) || strings.Contains(title, "MODELMETA_API_URL") {
		t.Fatalf("initial title exposed query content: %q", title)
	}
}
