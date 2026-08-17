package contextmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestReadContextArtifactReturnsCompleteOriginalContent(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{
		AgentRunID:             "run-artifact-read",
		ConfiguredAgentWindowK: 256,
		ModelContextWindow:     1_000_000,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("artifact evidence row 123456789\n", 2_000)
	hash := sha256.Sum256([]byte(content))
	hashText := hex.EncodeToString(hash[:])
	ref, err := store.Put(context.Background(), "run-artifact-read", hashText, content)
	if err != nil {
		t.Fatal(err)
	}
	manager.state.Messages = []adapter.Message{{
		Role:       "tool",
		ToolCallID: "search-1",
		Content:    `{"status":"compacted","tool_call_id":"search-1","artifact_ref":"` + ref + `","content_hash":"` + hashText + `","original_tokens":50000,"truncated":true}`,
	}}

	result, err := manager.ReadContextArtifact(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != content || result.ContentHash != hashText || result.SourceToolCallID != "search-1" {
		t.Fatalf("artifact result lost original identity: %#v", result)
	}
	if result.ReturnedTokens <= 0 || result.ReturnedTokens != result.TotalTokens {
		t.Fatalf("artifact tokens = returned:%d total:%d", result.ReturnedTokens, result.TotalTokens)
	}
	formatted, err := FormatContextArtifactToolResult(result)
	if err != nil {
		t.Fatal(err)
	}
	header, expanded, ok := decodeContextArtifactToolResult(formatted)
	if !ok || expanded != content || header["complete"] != true {
		t.Fatalf("formatted artifact result was not complete: header=%#v content_match=%t", header, expanded == content)
	}
}

func TestReadContextArtifactRejectsAnotherAgentRun(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{AgentRunID: "run-owner", ConfiguredAgentWindowK: 256, ModelContextWindow: 1_000_000}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := "private result"
	hash := sha256.Sum256([]byte(content))
	ref, err := store.Put(context.Background(), "run-other", hex.EncodeToString(hash[:]), content)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := manager.ReadContextArtifact(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "current Agent run") {
		t.Fatalf("cross-run read error = %v", err)
	}
}

func TestReadContextArtifactAllowsReferencedHistoricalRun(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{AgentRunID: "run-current", ConfiguredAgentWindowK: 256, ModelContextWindow: 1_000_000}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := "historical projected result"
	hash := sha256.Sum256([]byte(content))
	hashText := hex.EncodeToString(hash[:])
	ref, err := store.Put(context.Background(), "run-previous", hashText, content)
	if err != nil {
		t.Fatal(err)
	}
	manager.state.Messages = []adapter.Message{{
		Role:       "tool",
		ToolCallID: "historical-call",
		Content:    `{"status":"compacted","tool_call_id":"historical-call","artifact_ref":"` + ref + `","content_hash":"` + hashText + `"}`,
	}}

	result, err := manager.ReadContextArtifact(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != content || result.SourceToolCallID != "historical-call" {
		t.Fatalf("historical artifact result = %#v", result)
	}
}

func TestReadContextArtifactRejectsTamperedReference(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{AgentRunID: "run-artifact-validation", ConfiguredAgentWindowK: 256, ModelContextWindow: 1_000_000}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("validated content ", 100)
	hash := sha256.Sum256([]byte(content))

	for _, invalidRef := range []string{
		"file:///etc/passwd",
		"agent-context://tool-results/../" + hex.EncodeToString(hash[:]),
		"agent-context://tool-results/run-artifact-validation/not-a-hash",
	} {
		if _, err := manager.ReadContextArtifact(context.Background(), invalidRef); err == nil {
			t.Fatalf("invalid reference %q was accepted", invalidRef)
		}
	}
}
