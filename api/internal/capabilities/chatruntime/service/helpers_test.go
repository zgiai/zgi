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

func TestConversationTitleFallbackUsesFirstInputSingleLineTruncation(t *testing.T) {
	query := "  到文件管理\n读取第一个文件，然后根据设定续写一个很长很长的章节，并且把结果生成为精美的 PDF 文件保存起来  "

	title := conversationTitleFallback(query, defaultConversationTitle)

	if strings.ContainsAny(title, "\r\n\t") {
		t.Fatalf("conversationTitleFallback() returned a multi-line title: %q", title)
	}
	if got := len([]rune(title)); got != maxConversationTitleLen {
		t.Fatalf("conversationTitleFallback() rune count = %d, want %d; title=%q", got, maxConversationTitleLen, title)
	}
	if !strings.HasPrefix(title, "到文件管理 读取第一个文件") {
		t.Fatalf("conversationTitleFallback() = %q, want normalized first input prefix", title)
	}
}

func TestConversationTitleFallbackUsesDefaultForEmptyInput(t *testing.T) {
	if title := conversationTitleFallback(" \n\t ", defaultConversationTitle); title != defaultConversationTitle {
		t.Fatalf("conversationTitleFallback() = %q, want %q", title, defaultConversationTitle)
	}
}
