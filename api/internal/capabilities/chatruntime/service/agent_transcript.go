package service

import (
	"encoding/json"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	agentTranscriptMetadataKey        = "agent_transcript"
	agentTranscriptVersionMetadataKey = "agent_transcript_version"
	agentTranscriptSchemaVersion      = 1
)

func mergeAgentTranscriptMetadata(source map[string]interface{}, transcript []adapter.Message, answer string) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	normalized := normalizeAgentTranscript(transcript, answer)
	if len(normalized) == 0 {
		delete(metadata, agentTranscriptMetadataKey)
		delete(metadata, agentTranscriptVersionMetadataKey)
		return metadata
	}
	metadata[agentTranscriptVersionMetadataKey] = agentTranscriptSchemaVersion
	metadata[agentTranscriptMetadataKey] = normalized
	return metadata
}

func agentTranscriptFromMetadata(metadata map[string]interface{}, answer string) []adapter.Message {
	if len(metadata) == 0 {
		return nil
	}
	if version, ok := intValue(metadata["connector_history_version"]); ok && version == 1 {
		return nil
	}
	version, ok := intValue(metadata[agentTranscriptVersionMetadataKey])
	if !ok || version != agentTranscriptSchemaVersion {
		return nil
	}
	raw, ok := metadata[agentTranscriptMetadataKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var messages []adapter.Message
	if err := json.Unmarshal(encoded, &messages); err != nil {
		return nil
	}
	return normalizeAgentTranscript(messages, answer)
}

// normalizeAgentTranscript accepts only complete assistant tool-call batches.
// A crash can leave the final batch half-written; dropping that whole batch is
// safer than producing an invalid provider request with orphaned tool messages.
func normalizeAgentTranscript(source []adapter.Message, answer string) []adapter.Message {
	out := make([]adapter.Message, 0, len(source))
	seenCallIDs := make(map[string]struct{})
	for index := 0; index < len(source); {
		message := source[index]
		message.ReasoningContent = ""
		if !strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			index++
			continue
		}
		message.Role = "assistant"
		if len(message.ToolCalls) == 0 {
			if strings.TrimSpace(messageContentText(message.Content)) != "" {
				out = append(out, message)
			}
			index++
			continue
		}

		expected := make(map[string]struct{}, len(message.ToolCalls))
		validBatch := true
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" {
				validBatch = false
				break
			}
			if _, duplicate := expected[id]; duplicate {
				validBatch = false
				break
			}
			if _, duplicate := seenCallIDs[id]; duplicate {
				validBatch = false
				break
			}
			expected[id] = struct{}{}
		}

		results := make([]adapter.Message, 0, len(expected))
		found := make(map[string]struct{}, len(expected))
		next := index + 1
		for next < len(source) && strings.EqualFold(strings.TrimSpace(source[next].Role), "tool") {
			result := source[next]
			if content, changed := redactAttachmentReadContent(result.Content); changed {
				result.Content = content
			}
			result.ReasoningContent = ""
			result.Role = "tool"
			id := strings.TrimSpace(result.ToolCallID)
			if _, belongs := expected[id]; !belongs {
				validBatch = false
			} else if _, duplicate := found[id]; duplicate {
				validBatch = false
			} else {
				found[id] = struct{}{}
				results = append(results, result)
			}
			next++
		}
		if validBatch && len(found) == len(expected) {
			out = append(out, message)
			out = append(out, results...)
			for id := range expected {
				seenCallIDs[id] = struct{}{}
			}
		}
		index = next
	}

	answer = strings.TrimSpace(answer)
	if answer != "" && len(out) > 0 {
		last := out[len(out)-1]
		if strings.EqualFold(strings.TrimSpace(last.Role), "assistant") &&
			len(last.ToolCalls) == 0 {
			out = out[:len(out)-1]
		}
	}
	return out
}

func isUsableAgentTranscriptHistoryStatus(status string) bool {
	return isUsableAssistantHistoryStatus(status) || status == runtimemodel.MessageStatusError
}
