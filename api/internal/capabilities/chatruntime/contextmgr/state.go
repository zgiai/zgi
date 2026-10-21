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

// compactionBatch is the smallest safe unit for semantic-compaction
// boundaries. An assistant tool-call message and all immediately following
// tool results form one batch; every other message is a singleton batch.
// Keeping this unit intact prevents a summary request or a retry from
// producing an orphaned tool result.
type compactionBatch struct {
	Messages []adapter.Message
}

func groupMessagesByCompactionBatch(messages []adapter.Message) []compactionBatch {
	batches := make([]compactionBatch, 0, len(messages))
	for index := 0; index < len(messages); {
		end := index + 1
		message := messages[index]
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") && len(message.ToolCalls) > 0 {
			for end < len(messages) && strings.EqualFold(strings.TrimSpace(messages[end].Role), "tool") {
				end++
			}
		}
		batches = append(batches, compactionBatch{Messages: cloneMessages(messages[index:end])})
		index = end
	}
	return batches
}

func flattenCompactionBatches(batches []compactionBatch) []adapter.Message {
	messages := make([]adapter.Message, 0)
	for _, batch := range batches {
		messages = append(messages, cloneMessages(batch.Messages)...)
	}
	return messages
}

// selectPreservedTailMessages returns the first message of the verbatim tail.
// The count is measured in protocol messages, not user/assistant rounds. If
// the boundary falls inside a tool batch, the complete batch is retained and
// the returned tail is therefore allowed to exceed minimumMessages.
//
// A zero result means that there is no prefix to summarize (the complete
// dialogue is protected), which is important for short conversations.
func selectPreservedTailMessages(messages []adapter.Message, minimumMessages int) int {
	if minimumMessages <= 0 {
		minimumMessages = 1
	}
	if len(messages) <= minimumMessages {
		return 0
	}
	batches := groupMessagesByCompactionBatch(messages)
	messageCount := 0
	batchStart := len(batches)
	for batchStart > 0 && messageCount < minimumMessages {
		batchStart--
		messageCount += len(batches[batchStart].Messages)
	}
	if batchStart >= len(batches) {
		return 0
	}
	start := 0
	for index := 0; index < batchStart; index++ {
		start += len(batches[index].Messages)
	}
	return start
}

// selectPreservedTailMessagesByRounds returns the first message of the newest
// complete model rounds. A model round starts with an assistant response and
// includes every immediately following tool result. Any trailing current user
// input is preserved as part of the tail. Returning zero means there is no
// older prefix that can be summarized safely.
func selectPreservedTailMessagesByRounds(messages []adapter.Message, roundsToKeep int) int {
	if roundsToKeep <= 0 {
		roundsToKeep = defaultPreservedRounds
	}
	rounds := groupMessagesByAPIRound(messages)
	assistantRounds := make([]int, 0, len(rounds))
	for index, round := range rounds {
		for _, message := range round.Messages {
			if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
				assistantRounds = append(assistantRounds, index)
				break
			}
		}
	}
	if len(assistantRounds) <= roundsToKeep {
		return 0
	}
	firstPreservedRound := assistantRounds[len(assistantRounds)-roundsToKeep]
	start := 0
	for index := 0; index < firstPreservedRound; index++ {
		start += len(rounds[index].Messages)
	}
	return start
}

// groupMessagesByAPIRound starts a new group at every assistant response.
// Tool results following that assistant remain in the same group. The groups
// are retained for round-based diagnostics; compaction boundaries use the
// message-level batches above.
func groupMessagesByAPIRound(messages []adapter.Message) []apiRound {
	rounds := make([]apiRound, 0)
	current := apiRound{}
	sequence := 0
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		startsNewRound := false
		if len(current.Messages) > 0 && role == "assistant" {
			// Each assistant response is one model invocation. A follow-up
			// assistant response after tool results therefore starts a new round.
			for _, existing := range current.Messages {
				if strings.EqualFold(strings.TrimSpace(existing.Role), "assistant") {
					startsNewRound = true
					break
				}
			}
		}
		if len(current.Messages) > 0 && role == "user" {
			// A new user turn belongs to the next round, not to the previous
			// assistant response and its tool results.
			for _, existing := range current.Messages {
				if strings.EqualFold(strings.TrimSpace(existing.Role), "assistant") {
					startsNewRound = true
					break
				}
			}
		}
		if startsNewRound {
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

func summarizedEntryID(messages []adapter.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if id := strings.TrimSpace(messages[index].EntryID); id != "" {
			return id
		}
	}
	return ""
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
