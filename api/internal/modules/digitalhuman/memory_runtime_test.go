package digitalhuman

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRenderDigitalHumanMemoryContextDoesNotPreloadMemories(t *testing.T) {
	t.Parallel()
	memoryID := uuid.New()
	content := renderDigitalHumanMemoryContext([]*DigitalHumanMemory{{
		ID:        memoryID,
		MemoryKey: "launch-date", MemoryType: memoryruntime.MemoryTypeFact, Revision: 4, Content: "Launch is September 8.",
	}}, true, true)
	for _, unexpected := range []string{"id=" + memoryID.String(), "key=launch-date", "revision=4", "Launch is September 8"} {
		if strings.Contains(content, unexpected) {
			t.Errorf("context unexpectedly preloaded %q: %s", unexpected, content)
		}
	}
	if !strings.Contains(content, "retrieve_digital_human_memory") {
		t.Errorf("context missing retrieval guidance: %s", content)
	}
	for _, expected := range []string{"self/decision", "published identity", "Never store a conversational role directive as self/identity", "Decide at runtime", "not from trigger words", "Proactive memory maintenance is enabled", "memory_id, memory_key and revision of every match", "expected_revision 0", "as supersedes_id"} {
		if !strings.Contains(content, expected) {
			t.Errorf("context missing durable directive policy %q: %s", expected, content)
		}
	}
}

func TestDecodeMemoryRetrievalToolArgumentsBoundsAgentControls(t *testing.T) {
	t.Parallel()
	args, err := decodeMemoryRetrievalToolArguments(`{"query":"  launch owner  ","top_k":20,"retrieval_mode":"vector"}`)
	if err != nil || args.Query != "launch owner" || args.TopK != 20 || args.RetrievalMode != "vector" {
		t.Fatalf("retrieval args = %#v, err=%v", args, err)
	}
	if _, err := decodeMemoryRetrievalToolArguments(`{"query":"launch","top_k":21}`); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("top_k error = %v", err)
	}
	if _, err := decodeMemoryRetrievalToolArguments(`{"query":"launch","dataset_id":"secret"}`); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestDecodeMemoryToolArgumentsCarriesSupersedeFence(t *testing.T) {
	t.Parallel()
	targetID := uuid.New()
	args, err := decodeMemoryToolArguments(`{"action":"upsert","mode":"explicit","evidence":"Launch moved","key":"launch-v2","memory_type":"fact","content":"Launch moved.","expected_revision":0,"supersedes_id":"` + targetID.String() + `","expected_superseded_revision":4}`)
	if err != nil {
		t.Fatal(err)
	}
	if args.SupersedesID == nil || *args.SupersedesID != targetID || args.ExpectedSupersededRevision != 4 {
		t.Fatalf("supersede fence = %#v / %d", args.SupersedesID, args.ExpectedSupersededRevision)
	}
}

func TestMemoryToolOperationIDIsReplayStable(t *testing.T) {
	t.Parallel()
	runID := uuid.New()
	call := adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Name: digitalHumanMemoryToolName, Arguments: `{"key":"launch"}`}}
	first, second := memoryToolOperationID(runID, call), memoryToolOperationID(runID, call)
	if first == uuid.Nil || first != second {
		t.Fatalf("operation ids = %s / %s", first, second)
	}
	call.ID = "call-2"
	if memoryToolOperationID(runID, call) == first {
		t.Fatal("different call reused operation id")
	}
}

func TestValidateMemoryToolDecisionUsesModelModeAndExactEvidence(t *testing.T) {
	t.Parallel()
	query := "以后你就作为一个运营文案写手吧，主要职责是为产品输出运营软文"
	tests := []struct {
		name             string
		args             memoryToolArguments
		proactiveAllowed bool
		wantError        bool
	}{
		{name: "explicit durable instruction", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeExplicit, Evidence: "主要职责是为产品输出运营软文"}},
		{name: "ordinary content is not screened by code patterns", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeExplicit, Evidence: "以后你就作为"}},
		{name: "proactive upsert enabled", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeProactive, Evidence: "运营文案写手"}, proactiveAllowed: true},
		{name: "proactive disabled", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeProactive, Evidence: "运营文案写手"}, wantError: true},
		{name: "proactive clear rejected", args: memoryToolArguments{Action: memoryruntime.MutationActionClear, Mode: digitalHumanMemoryModeProactive, Evidence: "运营文案写手"}, proactiveAllowed: true, wantError: true},
		{name: "invented evidence rejected", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeExplicit, Evidence: "not in the message"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateMemoryToolDecision(test.args, query, test.proactiveAllowed)
			if (err != nil) != test.wantError {
				t.Fatalf("validateMemoryToolDecision() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestDecodeMemoryToolArgumentsRejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	_, err := decodeMemoryToolArguments(`{"action":"upsert","mode":"explicit","evidence":"launch","key":"launch","memory_type":"decision","expected_revision":0}{}`)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("decode error = %v, want invalid request", err)
	}
}

func TestSafeMemoryArtifactRefsKeepsIdentifiersOnly(t *testing.T) {
	t.Parallel()
	refs := safeMemoryArtifactRefs(map[string]interface{}{
		"conversation_artifacts": []interface{}{map[string]interface{}{
			"artifact_id": "artifact-1", "artifact_type": "report", "file_id": "file-1",
			"content": "must-not-persist", "token": "secret",
		}},
	})
	value := string(refs)
	for _, expected := range []string{"artifact-1", "report", "file-1"} {
		if !strings.Contains(value, expected) {
			t.Errorf("artifact refs missing %q: %s", expected, value)
		}
	}
	for _, forbidden := range []string{"must-not-persist", "secret", "content", "token"} {
		if strings.Contains(value, forbidden) {
			t.Errorf("artifact refs contain %q: %s", forbidden, value)
		}
	}
}
