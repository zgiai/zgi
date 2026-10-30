package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Write-time compression budget for one transcript entry. Tool results are
// compressed only when the context manager has already crossed its global
// threshold; this writer applies the same lossy representation when durable
// history is first written so payload and preview cannot diverge.

// transcriptEntryKindForRole maps a protocol role onto a stored entry kind.
// System messages are never stored: the prologue is re-rendered per request.
func transcriptEntryKindForRole(role string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return runtimemodel.TranscriptEntryKindUser, true
	case "assistant":
		return runtimemodel.TranscriptEntryKindAssistant, true
	case "tool":
		return runtimemodel.TranscriptEntryKindTool, true
	default:
		return "", false
	}
}

// protocolMessageToJSON renders one protocol message into its durable JSON form.
func protocolMessageToJSON(message adapter.Message) (map[string]interface{}, error) {
	if message.Role == "tool" {
		if content, changed := redactAttachmentReadContent(message.Content); changed {
			message.Content = content
		}
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode protocol message: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, fmt.Errorf("decode protocol message: %w", err)
	}
	return payload, nil
}

// protocolMessageFromJSON restores the protocol message stored in an entry.
// Reasoning content is dropped: it is replayed per provider policy, never from
// the durable log.
func protocolMessageFromJSON(payload map[string]interface{}) (adapter.Message, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, fmt.Errorf("encode stored protocol message: %w", err)
	}
	var message adapter.Message
	if err := json.Unmarshal(encoded, &message); err != nil {
		return adapter.Message{}, fmt.Errorf("decode stored protocol message: %w", err)
	}
	if !adapter.ReasoningHistoryKept() {
		message.ReasoningContent = ""
	}
	return message, nil
}

// protocolMessageFingerprint identifies one protocol message for boundary
// matching. It is computed from the same JSON that is stored, so the live
// request and the durable entry agree.
func protocolMessageFingerprint(payload map[string]interface{}) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// pruneTranscriptMessageForWrite computes the canonical form stored in both
// preview and payload. Oversized tool results keep the beginning and end only;
// the omitted middle is intentionally unrecoverable.
func pruneTranscriptMessageForWrite(entryID uuid.UUID, payload map[string]interface{}, kind string) map[string]interface{} {
	if kind != runtimemodel.TranscriptEntryKindTool {
		return payload
	}
	if content, changed := redactAttachmentReadContent(payload["content"]); changed {
		out := copyStringAnyMap(payload)
		out["content"] = content
		return out
	}
	text := messageContentText(payload["content"])
	// The context manager is the sole component that decides when and how to
	// compress. Persistence must not apply an independent size threshold.
	_ = entryID
	_ = text
	return payload
}

// Attachment pages are an explicit ephemeral capability, not context-manager
// compaction. Persist their range/version receipt, never the retrieved body.
func redactAttachmentReadContent(content interface{}) (interface{}, bool) {
	text, ok := content.(string)
	if !ok {
		return content, false
	}
	var receipt map[string]interface{}
	if json.Unmarshal([]byte(text), &receipt) != nil || receipt["source"] != "attachment_reader" || receipt["content_redacted_in_history"] != true {
		return content, false
	}
	if _, exists := receipt["content"]; !exists {
		return content, false
	}
	delete(receipt, "content")
	receipt["content_redacted"] = true
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return content, false
	}
	return string(encoded), true
}

// projectContextMessages decodes a chronological list of transcript previews.
// Summary entries are projected by the parent-chain loader and are ignored here.
func projectContextMessages(entries []*runtimemodel.TranscriptEntryPreview) ([]adapter.Message, error) {
	messages := make([]adapter.Message, 0, len(entries)+1)
	for _, entry := range entries {
		if entry == nil || entry.Kind == runtimemodel.TranscriptEntryKindSummary || len(entry.Preview) == 0 {
			continue
		}
		message, err := protocolMessageFromJSON(entry.Preview)
		if err != nil {
			return nil, fmt.Errorf("project transcript entry %s: %w", entry.ID, err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

// snapContextWindowStart moves a retention-window start backwards to the
// nearest legal protocol boundary. A window must never open with a tool result
// whose assistant tool_call fell outside it, and it must never cut a tool-call
// batch in half, so the window may grow but not shrink.
func snapContextWindowStart(messages []adapter.Message, start int) int {
	if start <= 0 {
		return 0
	}
	if start > len(messages) {
		start = len(messages)
	}
	for start > 0 && start < len(messages) &&
		strings.EqualFold(strings.TrimSpace(messages[start].Role), "tool") {
		start--
	}
	return start
}

// closeDanglingToolCalls makes a projected message list provider-valid by
// closing every assistant tool call that has no recorded result. It returns the
// closed list plus the synthetic results, so the caller can persist them as
// real entries instead of re-synthesizing them on the next replay.
//
// A synthetic result never claims success: it tells the model the outcome was
// not recorded and that a side-effecting call must not be retried blindly.
func closeDanglingToolCalls(messages []adapter.Message) ([]adapter.Message, []adapter.Message) {
	if len(messages) == 0 {
		return messages, nil
	}
	closed := make([]adapter.Message, 0, len(messages))
	var synthesized []adapter.Message
	for index := 0; index < len(messages); index++ {
		message := messages[index]
		closed = append(closed, message)
		if !strings.EqualFold(strings.TrimSpace(message.Role), "assistant") || len(message.ToolCalls) == 0 {
			continue
		}
		recorded := make(map[string]struct{}, len(message.ToolCalls))
		next := index + 1
		for next < len(messages) && strings.EqualFold(strings.TrimSpace(messages[next].Role), "tool") {
			if id := strings.TrimSpace(messages[next].ToolCallID); id != "" {
				recorded[id] = struct{}{}
			}
			closed = append(closed, messages[next])
			next++
		}
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" {
				continue
			}
			if _, ok := recorded[id]; ok {
				continue
			}
			result := adapter.Message{
				Role:       "tool",
				ToolCallID: id,
				Content:    danglingToolCallResultContent(call.Function.Name),
			}
			closed = append(closed, result)
			synthesized = append(synthesized, result)
		}
		index = next - 1
	}
	return closed, synthesized
}

func danglingToolCallResultContent(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "该工具"
	}
	return fmt.Sprintf(
		"[结果未记录] %s 的调用已发起，但执行结果没有落到上下文日志里（进程中断或写入失败）。不要盲目重试：如果该操作可能产生副作用，请先核实外部状态，或向用户确认后再决定。",
		name,
	)
}
