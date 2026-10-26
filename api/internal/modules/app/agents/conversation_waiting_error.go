package agents

import (
	"errors"
	"net/http"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// The waiting states an external caller can meet when it starts a new turn,
// each with a message saying how to continue.
var (
	AppCodeAgentConversationWaitingApproval = apperror.MustCode("agent.conversation.waiting_approval")
	AppCodeAgentConversationWaitingQuestion = apperror.MustCode("agent.conversation.waiting_question")
	AppCodeAgentConversationWaitingAction   = apperror.MustCode("agent.conversation.waiting_action")
	AppCodeAgentConversationWaitingBudget   = apperror.MustCode("agent.conversation.waiting_budget")
)

func conversationWaitingDefinitions() []appcatalog.Definition {
	definition := func(code apperror.Code, english, chinese string) appcatalog.Definition {
		return appcatalog.Definition{
			Code: code, Category: appcatalog.CategoryConflict, HTTPStatus: http.StatusConflict, Retryable: false,
			Messages: map[appcatalog.Locale]string{appcatalog.LocaleEnglishUS: english, appcatalog.LocaleChineseSimplified: chinese},
		}
	}
	return []appcatalog.Definition{
		definition(AppCodeAgentConversationWaitingApproval,
			"The conversation is waiting for an approval. Continue its latest message once it is approved.",
			"会话正在等待审批，审批后请继续最新一条消息。"),
		definition(AppCodeAgentConversationWaitingQuestion,
			"The conversation is waiting for an answer to its question. Answer it to continue.",
			"会话正在等待回答一个问题，请先回答再继续。"),
		definition(AppCodeAgentConversationWaitingAction,
			"The conversation is waiting for a client action. Submit its result to continue.",
			"会话正在等待客户端操作，请提交操作结果后继续。"),
		definition(AppCodeAgentConversationWaitingBudget,
			"The conversation paused at its token budget. Continue its latest message to go on.",
			"会话已达到本轮 Token 预算并暂停，请继续最新一条消息。"),
	}
}

// conversationWaitingCode maps a waiting error to its public definition.
func conversationWaitingCode(err error) apperror.Code {
	switch {
	case errors.Is(err, runtimeservice.ErrConversationWaitingApproval):
		return AppCodeAgentConversationWaitingApproval
	case errors.Is(err, runtimeservice.ErrConversationWaitingQuestion):
		return AppCodeAgentConversationWaitingQuestion
	case errors.Is(err, runtimeservice.ErrConversationWaitingAction):
		return AppCodeAgentConversationWaitingAction
	default:
		return AppCodeAgentConversationWaitingBudget
	}
}

// conversationWaitingMessage is the localized message for a waiting error.
// Without a catalog it falls back to the English definition, never to the
// internal error text.
func conversationWaitingMessage(projector *apptransport.Projector, err error, locale appcatalog.Locale) string {
	code := conversationWaitingCode(err)
	if projector != nil {
		if message := projector.Project(apperror.Wrap(err, code), locale).Presentation.Message; message != "" {
			return message
		}
	}
	for _, definition := range conversationWaitingDefinitions() {
		if definition.Code == code {
			return definition.Messages[appcatalog.LocaleEnglishUS]
		}
	}
	return ""
}
