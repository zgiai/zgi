package service

import (
	"strings"
	"testing"

	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// The interface language is named with the rule that writing in another
// language does not change it; a run without one gets no language line
// (ADR 0048).
func TestUserLanguageContextNamesTheInterfaceLanguage(t *testing.T) {
	text := userLanguageContext(appcatalog.LocaleChineseSimplified)
	if !strings.Contains(text, "Simplified Chinese (zh-Hans)") || !strings.Contains(text, "also when the user writes in another language") {
		t.Fatalf("userLanguageContext(zh-Hans) = %q", text)
	}
	if !strings.Contains(userLanguageContext(appcatalog.LocaleEnglishUS), "English (en-US)") {
		t.Fatal("userLanguageContext(en-US) does not name English")
	}
	if userLanguageContext("") != "" {
		t.Fatal("a run without a language got a language line")
	}
}

// The language line follows the request, where models read it last.
func TestQueryWithLanguageReminderFollowsTheRequest(t *testing.T) {
	parts := &chatRequestParts{UserLanguageContext: userLanguageContext(appcatalog.LocaleChineseSimplified)}
	got := queryWithLanguageReminder(parts, "What is 15% of 2400?")
	if !strings.HasPrefix(got, "What is 15% of 2400?\n\n") || !strings.HasSuffix(strings.TrimSpace(got), "</system-reminder>") {
		t.Fatalf("queryWithLanguageReminder() = %q", got)
	}
	if got := queryWithLanguageReminder(&chatRequestParts{}, "hi"); got != "hi" {
		t.Fatalf("queryWithLanguageReminder() without a language = %q", got)
	}
}
