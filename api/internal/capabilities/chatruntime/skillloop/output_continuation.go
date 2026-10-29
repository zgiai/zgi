package skillloop

import (
	"context"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// maxOutputContinuations caps how often one round continues an output that
// reached the model's output limit (ADR 0041).
const maxOutputContinuations = 2

const (
	continueAnswerNotice = "Your previous message was cut off by the output limit. Continue it exactly where it stopped. Do not repeat or summarize what you already wrote."
	retryToolCallNotice  = "Your previous response was cut off by the output limit before it was complete, so it was discarded. Send it again more compactly: put large content in several smaller tool calls, and keep any text short."
)

// runNativeAgentRoundContinuing runs one native round and, when the output
// stops at the model's output limit, continues it instead of failing the turn
// (ADR 0041):
//
//   - an answer cut off as text is continued from where it stopped, and the
//     parts are joined;
//   - a response cut off inside a tool call, or before any output, is sent
//     again with a larger output limit and a request to be compact.
func (r *Runner) runNativeAgentRoundContinuing(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, terminalProtocol bool, terminalStreamingAllowed bool, suppressNaturalProgress bool, outputTokenLimit int) (planningResult, error) {
	result, err := r.runNativeAgentRound(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress)
	contextMessages := result.contextMessages
	// A max_tokens set in the model parameters limits the answer, so an
	// answer that reaches it is not continued past it.
	configuredLimit := planningReq.MaxTokens != nil && *planningReq.MaxTokens > 0
	for attempt := 1; attempt <= maxOutputContinuations && isPlanningOutputLimitError(err); attempt++ {
		partial := result.message
		text := assistantMessageText(partial)
		continuing := len(normalizeToolCalls(partial.ToolCalls)) == 0 && strings.TrimSpace(text) != ""
		if continuing && configuredLimit {
			break
		}
		next := cloneChatRequest(planningReq)
		var appended []adapter.Message
		if continuing {
			appended = []adapter.Message{
				{Role: "assistant", Content: text, ReasoningContent: partial.ReasoningContent},
				{Role: "system", Content: continueAnswerNotice},
			}
		} else {
			current := planningReq.MaxTokens
			if effective := r.diagnostics.requestBudget.effectiveMaxTokens; effective > 0 {
				current = &effective
			}
			maxTokens := planningRetryMaxTokens(current, outputTokenLimit)
			next.MaxTokens = &maxTokens
			appended = []adapter.Message{{Role: "system", Content: retryToolCallNotice}}
		}
		next.Messages = append(append([]adapter.Message{}, planningReq.Messages...), appended...)
		logger.WarnContext(ctx, "chat runtime continuing output cut off by the output limit",
			"message_id", prepared.Message.ID.String(),
			"round", round,
			"attempt", attempt,
			"continuing_answer", continuing,
		)
		nextResult, nextErr := r.runNativeAgentRound(ctx, prepared, next, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress)
		nextResult.usage = mergeUsage(result.usage, nextResult.usage)
		if continuing {
			nextText := assistantMessageText(nextResult.message)
			if result.answerStreamed && !nextResult.answerStreamed && len(normalizeToolCalls(nextResult.message.ToolCalls)) == 0 && nextText != "" {
				r.emitAnswerChunk(ctx, prepared, nextText, nil)
				nextResult.answerStreamed = true
			}
			nextResult.message.Content = text + nextText
			nextResult.message.ReasoningContent = joinReasoning(partial.ReasoningContent, nextResult.message.ReasoningContent)
			nextResult.provisionalContent = result.provisionalContent + nextResult.provisionalContent
		}
		// Later rounds build on the request as it was, without the cut-off
		// output and the notice.
		nextResult.contextMessages = withoutAppendedMessages(nextResult.contextMessages, appended, contextMessages)
		result, err = nextResult, nextErr
	}
	return result, err
}

func joinReasoning(first, second string) string {
	switch {
	case strings.TrimSpace(first) == "":
		return second
	case strings.TrimSpace(second) == "":
		return first
	default:
		return first + "\n" + second
	}
}

// withoutAppendedMessages removes the continuation messages from the end of
// the messages a round sent. If the context manager changed the tail, the
// messages of the first attempt are used instead.
func withoutAppendedMessages(messages, appended, fallback []adapter.Message) []adapter.Message {
	if len(messages) < len(appended) {
		return fallback
	}
	// A system notice reaches the provider as a reminder in a user message
	// (ADR 0032), so the tail is matched by content.
	tail := messages[len(messages)-len(appended):]
	for index := range appended {
		if !strings.Contains(assistantMessageText(tail[index]), assistantMessageText(appended[index])) {
			return fallback
		}
	}
	return messages[:len(messages)-len(appended)]
}
