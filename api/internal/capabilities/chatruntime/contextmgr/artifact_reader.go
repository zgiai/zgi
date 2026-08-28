package contextmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const contextArtifactScheme = "agent-context"
const contextArtifactHost = "tool-results"
const contextArtifactContentMarker = "\n\n[artifact content]\n"
const contextArtifactReadKind = "context_artifact_read"

// ReadContextArtifact returns the complete immutable tool result. The caller
// must run the resulting model request through the normal context budget; this
// method deliberately does not truncate or page artifact content.
func (m *Manager) ReadContextArtifact(ctx context.Context, artifactRef string) (ContextArtifactResult, error) {
	if m == nil || m.toolStore == nil {
		return ContextArtifactResult{}, fmt.Errorf("context artifact storage is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ContextArtifactResult{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	runPart, contentHash, err := parseContextArtifactRef(artifactRef)
	if err != nil {
		return ContextArtifactResult{}, err
	}
	expectedRunPart, err := safePathPart(m.state.AgentRunID)
	if err != nil {
		return ContextArtifactResult{}, err
	}
	if runPart != expectedRunPart && !artifactRefAppearsInMessages(artifactRef, m.state.Messages) {
		return ContextArtifactResult{}, fmt.Errorf("context artifact does not belong to the current Agent run")
	}

	content, err := m.toolStore.Get(ctx, runPart, contentHash)
	if err != nil {
		return ContextArtifactResult{}, err
	}
	digest := sha256.Sum256([]byte(content))
	if hex.EncodeToString(digest[:]) != contentHash {
		return ContextArtifactResult{}, fmt.Errorf("context artifact integrity check failed")
	}
	totalTokens := m.estimator.EstimateText(content, strings.TrimSpace(m.state.Model)).Tokens
	return ContextArtifactResult{
		ArtifactRef:      strings.TrimSpace(artifactRef),
		ContentHash:      contentHash,
		SourceToolCallID: m.sourceToolCallIDForArtifact(artifactRef),
		Content:          content,
		ReturnedTokens:   totalTokens,
		TotalTokens:      totalTokens,
	}, nil
}

func artifactRefAppearsInMessages(artifactRef string, messages []adapter.Message) bool {
	artifactRef = strings.TrimSpace(artifactRef)
	if artifactRef == "" {
		return false
	}
	for _, message := range messages {
		content := contentString(message.Content)
		if strings.Contains(content, artifactRef) {
			return true
		}
		if receipt, ok := decodeToolResultReceipt(content); ok && strings.TrimSpace(fmt.Sprint(receipt["artifact_ref"])) == artifactRef {
			return true
		}
	}
	return false
}

func (m *Manager) sourceToolCallIDForArtifact(artifactRef string) string {
	artifactRef = strings.TrimSpace(artifactRef)
	for index := len(m.state.Messages) - 1; index >= 0; index-- {
		message := m.state.Messages[index]
		receipt, ok := decodeToolResultReceipt(contentString(message.Content))
		if !ok || strings.TrimSpace(fmt.Sprint(receipt["artifact_ref"])) != artifactRef {
			continue
		}
		if toolCallID := strings.TrimSpace(fmt.Sprint(receipt["tool_call_id"])); toolCallID != "" && toolCallID != "<nil>" {
			return toolCallID
		}
		if toolCallID := strings.TrimSpace(message.ToolCallID); toolCallID != "" {
			return toolCallID
		}
	}
	return ""
}

// FormatContextArtifactToolResult marks expanded artifact content so context
// management can keep it verbatim until ordinary microcompaction selects it,
// then reuse the original artifact instead of recursively persisting a copy.
func FormatContextArtifactToolResult(result ContextArtifactResult) (string, error) {
	header := map[string]interface{}{
		"status":          "success",
		"kind":            contextArtifactReadKind,
		"artifact_ref":    strings.TrimSpace(result.ArtifactRef),
		"content_hash":    strings.TrimSpace(result.ContentHash),
		"returned_tokens": result.ReturnedTokens,
		"total_tokens":    result.TotalTokens,
		"complete":        true,
	}
	if sourceToolCallID := strings.TrimSpace(result.SourceToolCallID); sourceToolCallID != "" {
		header["source_tool_call_id"] = sourceToolCallID
	}
	encoded, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	return string(encoded) + contextArtifactContentMarker + result.Content, nil
}

func decodeContextArtifactToolResult(content string) (map[string]interface{}, string, bool) {
	parts := strings.SplitN(content, contextArtifactContentMarker, 2)
	if len(parts) != 2 {
		return nil, "", false
	}
	header := map[string]interface{}{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(parts[0])), &header); err != nil {
		return nil, "", false
	}
	if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(header["status"])), "success") ||
		!strings.EqualFold(strings.TrimSpace(fmt.Sprint(header["kind"])), contextArtifactReadKind) ||
		strings.TrimSpace(fmt.Sprint(header["artifact_ref"])) == "" ||
		strings.TrimSpace(fmt.Sprint(header["content_hash"])) == "" {
		return nil, "", false
	}
	return header, parts[1], true
}

func parseContextArtifactRef(artifactRef string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(artifactRef))
	if err != nil || parsed.Scheme != contextArtifactScheme || parsed.Host != contextArtifactHost || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", "", fmt.Errorf("invalid context artifact reference")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid context artifact reference")
	}
	runPart, err := safePathPart(parts[0])
	if err != nil || runPart != parts[0] {
		return "", "", fmt.Errorf("invalid context artifact run identifier")
	}
	hashPart, err := safePathPart(parts[1])
	if err != nil || hashPart != parts[1] {
		return "", "", fmt.Errorf("invalid context artifact content hash")
	}
	decodedHash, err := hex.DecodeString(hashPart)
	if err != nil || len(decodedHash) != sha256.Size {
		return "", "", fmt.Errorf("invalid context artifact content hash")
	}
	return runPart, hashPart, nil
}
