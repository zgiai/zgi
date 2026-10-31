package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// applyUserLanguageContext tells a run the user's interface language (ADR
// 0048): the account's language in a console conversation, the language the
// request asks for otherwise. Channel and task runs serve people the runtime
// has no language for, so they get none and the model follows the user's
// writing.
func (s *service) applyUserLanguageContext(ctx context.Context, scope Scope, parts *chatRequestParts, conversationID uuid.UUID) {
	if parts == nil {
		return
	}
	parts.UserLanguageContext = userLanguageContext(s.userLanguage(ctx, scope, conversationID))
}

func (s *service) userLanguage(ctx context.Context, scope Scope, conversationID uuid.UUID) appcatalog.Locale {
	source := runtimemodel.ConversationSourceConsole
	if s.repos != nil && s.repos.DB != nil && conversationID != uuid.Nil {
		var stored string
		if err := s.repos.DB.WithContext(ctx).Table("chat_runtime_conversations").Select("source").Where("id = ?", conversationID).Scan(&stored).Error; err == nil && strings.TrimSpace(stored) != "" {
			source = strings.TrimSpace(stored)
		}
	}
	switch source {
	case runtimemodel.ConversationSourceConsole, runtimemodel.ConversationSourcePreview:
		if s.repos != nil && s.repos.DB != nil && scope.AccountID != uuid.Nil {
			var stored string
			_ = s.repos.DB.WithContext(ctx).Table("accounts").Select("interface_language").Where("id = ?", scope.AccountID).Scan(&stored).Error
			if locale, ok := appcatalog.ParseLocale(stored); ok {
				return locale
			}
		}
		return StreamLocale(ctx)
	case runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI:
		return StreamLocale(ctx)
	default:
		return ""
	}
}

func userLanguageContext(locale appcatalog.Locale) string {
	var name string
	switch locale {
	case appcatalog.LocaleChineseSimplified:
		name = "Simplified Chinese"
	case appcatalog.LocaleEnglishUS:
		name = "English"
	default:
		return ""
	}
	return "The user's interface language is " + name + " (" + string(locale) + "). Write notes, questions and answers in " + name + ", also when the user writes in another language, unless the user asks for a specific language."
}

// queryWithLanguageReminder puts the interface language after the user's
// request. Models follow the language of the text they read last; before the
// request, Kimi answered an English question in English despite it (ADR 0048).
func queryWithLanguageReminder(parts *chatRequestParts, query string) string {
	if parts == nil || strings.TrimSpace(parts.UserLanguageContext) == "" || strings.TrimSpace(query) == "" {
		return query
	}
	return query + "\n\n" + adapter.SystemReminder(parts.UserLanguageContext)
}
