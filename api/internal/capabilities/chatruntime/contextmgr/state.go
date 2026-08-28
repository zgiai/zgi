package contextmgr

import (
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type apiRound struct {
	Sequence int
	Messages []adapter.Message
}

// groupMessagesByAPIRound starts a new group at every assistant response.
// Tool results following that assistant remain in the same group, so a
// compaction boundary can never split a tool_call/tool_result pair.
func groupMessagesByAPIRound(messages []adapter.Message) []apiRound {
	rounds := make([]apiRound, 0)
	current := apiRound{}
	sequence := 0
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") && len(current.Messages) > 0 {
			rounds = append(rounds, current)
			sequence++
			current = apiRound{Sequence: sequence}
		}
		current.Messages = append(current.Messages, cloneMessage(message))
	}
	if len(current.Messages) > 0 {
		rounds = append(rounds, current)
	}
	return rounds
}

// groupMessagesByAPIRoundForRun assigns absolute run-local sequence numbers to
// the assistant-led groups at the tail. Earlier assistant groups can belong to
// bootstrap conversation history and intentionally retain sequence 0.
func groupMessagesByAPIRoundForRun(messages []adapter.Message, nextRound int) []apiRound {
	rounds := groupMessagesByAPIRound(messages)
	for index := range rounds {
		rounds[index].Sequence = 0
	}
	completedRounds := max(0, nextRound-1)
	assistantGroups := make([]int, 0, len(rounds))
	for index, round := range rounds {
		for _, message := range round.Messages {
			if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
				assistantGroups = append(assistantGroups, index)
				break
			}
		}
	}
	assigned := min(completedRounds, len(assistantGroups))
	firstSequence := completedRounds - assigned + 1
	for offset := 0; offset < assigned; offset++ {
		groupIndex := assistantGroups[len(assistantGroups)-assigned+offset]
		rounds[groupIndex].Sequence = firstSequence + offset
	}
	return rounds
}

func validateToolPairing(messages []adapter.Message) error {
	toolCalls := map[string]struct{}{}
	toolResults := map[string]struct{}{}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" {
				return fmt.Errorf("tool call is missing id")
			}
			if _, exists := toolCalls[id]; exists {
				return fmt.Errorf("duplicate tool call id %q", id)
			}
			toolCalls[id] = struct{}{}
		}
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		id := strings.TrimSpace(message.ToolCallID)
		if id == "" {
			return fmt.Errorf("tool result is missing tool_call_id")
		}
		if _, ok := toolCalls[id]; !ok {
			return fmt.Errorf("tool result %q has no matching tool call", id)
		}
		if _, exists := toolResults[id]; exists {
			return fmt.Errorf("tool call %q has multiple tool results", id)
		}
		toolResults[id] = struct{}{}
	}
	for id := range toolCalls {
		if _, ok := toolResults[id]; !ok {
			return fmt.Errorf("tool call %q has no tool result", id)
		}
	}
	return nil
}

func cloneMessages(messages []adapter.Message) []adapter.Message {
	out := make([]adapter.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, cloneMessage(message))
	}
	return out
}

func cloneMessage(message adapter.Message) adapter.Message {
	message.ToolCalls = append([]adapter.ToolCall(nil), message.ToolCalls...)
	if parts, ok := message.Content.([]adapter.MessageContentPart); ok {
		message.Content = append([]adapter.MessageContentPart(nil), parts...)
	}
	return message
}

func cloneRequest(request *adapter.ChatRequest) *adapter.ChatRequest {
	if request == nil {
		return nil
	}
	cloned := *request
	cloned.Messages = cloneMessages(request.Messages)
	cloned.Tools = append([]adapter.Tool(nil), request.Tools...)
	cloned.Functions = append([]adapter.Function(nil), request.Functions...)
	cloned.Stop = append([]string(nil), request.Stop...)
	if request.AdditionalParameters != nil {
		cloned.AdditionalParameters = make(map[string]interface{}, len(request.AdditionalParameters))
		for key, value := range request.AdditionalParameters {
			cloned.AdditionalParameters[key] = value
		}
	}
	return &cloned
}
