package digitalhuman

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAuthorizedMemoryScopesRequireTrustedUpstreamProjection(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.New()
	event := invocation.Event{Context: map[string]any{
		"memory_scopes": []interface{}{map[string]any{"type": "project", "id": "project-7"}},
	}}
	if scopes := authorizedMemoryScopes(event, workspaceID); len(scopes) != 1 || scopes[0].Type != memoryruntime.MemoryScopeWorkspace {
		t.Fatalf("untrusted scopes = %#v", scopes)
	}
	event.Context["memory_scopes_authorized"] = true
	if scopes := authorizedMemoryScopes(event, workspaceID); len(scopes) != 2 || scopes[1].Type != memoryruntime.MemoryScopeProject || scopes[1].ID != "project-7" {
		t.Fatalf("trusted scopes = %#v", scopes)
	}
}

func TestMemoryToolBusinessScopeRequiresExactAuthorization(t *testing.T) {
	t.Parallel()
	authorized := []memoryruntime.MemoryScopeRef{
		{Type: memoryruntime.MemoryScopeWorkspace, ID: "workspace-1"},
		{Type: memoryruntime.MemoryScopeProject, ID: "project-7"},
	}
	base := memoryToolArguments{
		MemoryClass: memoryruntime.MemoryClassBusiness,
		ScopeType:   memoryruntime.MemoryScopeProject,
		ScopeID:     "project-7",
	}
	if !memoryToolBusinessScopeAuthorized(base, authorized) {
		t.Fatal("exact authorized business scope was rejected")
	}
	base.ScopeID = "project-8"
	if memoryToolBusinessScopeAuthorized(base, authorized) {
		t.Fatal("different project scope was authorized")
	}
	base.ScopeType, base.ScopeID = memoryruntime.MemoryScopeCustomer, "project-7"
	if memoryToolBusinessScopeAuthorized(base, authorized) {
		t.Fatal("same identifier under a different scope type was authorized")
	}
	base.MemoryClass = memoryruntime.MemoryClassSelf
	if !memoryToolBusinessScopeAuthorized(base, authorized) {
		t.Fatal("self memory was incorrectly subjected to business scope authorization")
	}
}

func TestRenderDigitalHumanMemoryContextCarriesScopeAndRevision(t *testing.T) {
	t.Parallel()
	memoryID := uuid.New()
	content := renderDigitalHumanMemoryContext([]*DigitalHumanMemory{{
		ID:        memoryID,
		MemoryKey: "launch-date", MemoryClass: memoryruntime.MemoryClassBusiness, MemoryType: memoryruntime.MemoryTypeFact,
		ScopeType: memoryruntime.MemoryScopeProject, ScopeID: "project-7", Revision: 4, Content: "Launch is September 8.",
	}}, true, true)
	for _, expected := range []string{"id=" + memoryID.String(), "key=launch-date", "scope=project:project-7", "revision=4", "Launch is September 8"} {
		if !strings.Contains(content, expected) {
			t.Errorf("context missing %q: %s", expected, content)
		}
	}
	for _, expected := range []string{"self/decision", "published identity", "Never store a conversational role directive as self/identity", "Decide at runtime", "not from trigger words", "Proactive memory maintenance is enabled"} {
		if !strings.Contains(content, expected) {
			t.Errorf("context missing durable directive policy %q: %s", expected, content)
		}
	}
}

func TestDecodeMemoryToolArgumentsCarriesSupersedeFence(t *testing.T) {
	t.Parallel()
	targetID := uuid.New()
	args, err := decodeMemoryToolArguments(`{"action":"upsert","mode":"explicit","evidence":"Launch moved","key":"launch-v2","memory_class":"business","memory_type":"fact","scope_type":"project","scope_id":"project-7","content":"Launch moved.","expected_revision":0,"supersedes_id":"` + targetID.String() + `","expected_superseded_revision":4}`)
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

func TestDigitalHumanMemoryToolAvailabilityIsPolicyDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		origin          invocation.OriginKind
		autoExtraction  bool
		actorAuthorized bool
		want            bool
	}{
		{name: "authorized manual run", origin: invocation.OriginManual, actorAuthorized: true, want: true},
		{name: "manual run requires actor permission", origin: invocation.OriginManual, autoExtraction: true, want: false},
		{name: "automatic channel maintenance", origin: invocation.OriginChannel, autoExtraction: true, want: true},
		{name: "authorized channel explicit maintenance", origin: invocation.OriginChannel, actorAuthorized: true, want: true},
		{name: "unconfigured channel maintenance", origin: invocation.OriginChannel, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state := &MemoryState{AutoExtractionEnabled: test.autoExtraction}
			event := invocation.Event{Origin: invocation.Origin{Kind: test.origin}}
			if got := digitalHumanMemoryToolEnabled(event, state, test.actorAuthorized); got != test.want {
				t.Fatalf("digitalHumanMemoryToolEnabled(%s, auto=%v, authorized=%v) = %v, want %v", test.origin, test.autoExtraction, test.actorAuthorized, got, test.want)
			}
		})
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
	_, err := decodeMemoryToolArguments(`{"action":"upsert","mode":"explicit","evidence":"launch","key":"launch","memory_class":"self","memory_type":"decision","expected_revision":0}{}`)
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
