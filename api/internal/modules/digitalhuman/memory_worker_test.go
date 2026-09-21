package digitalhuman

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/datatypes"
)

func TestParseMemoryExtractionDraftsNormalizesStableKeys(t *testing.T) {
	t.Parallel()
	targetID := uuid.New()
	drafts, err := parseMemoryExtractionDrafts(`{"proposals":[{"key":" Launch-Date ","memory_class":"business","memory_type":"fact","scope_type":"project","scope_id":"project-7","content":"Launch is September 8.","confidence":0.9,"supersedes_id":"` + targetID.String() + `","expected_superseded_revision":3}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].Key != "launch-date" || drafts[0].ScopeID != "project-7" || drafts[0].Confidence != 0.9 ||
		drafts[0].SupersedesID == nil || *drafts[0].SupersedesID != targetID || drafts[0].ExpectedSupersededRevision != 3 {
		t.Fatalf("drafts = %#v", drafts)
	}
}

func TestParseMemoryExtractionDraftsRejectsDuplicateIdentity(t *testing.T) {
	t.Parallel()
	_, err := parseMemoryExtractionDrafts(`{"proposals":[
		{"key":"launch","memory_class":"self","memory_type":"decision","content":"A"},
		{"key":"launch","memory_class":"self","memory_type":"decision","content":"B"}
	]}`)
	if err == nil {
		t.Fatal("duplicate proposal was accepted")
	}
}

func TestMemoryExtractionPromptContainsReferencesWithoutArtifactBodies(t *testing.T) {
	t.Parallel()
	memoryID := uuid.New()
	input := MemoryExtractionInput{
		Job: &MemoryExtractionJob{
			ConversationID: ptrUUID(uuid.New()),
			ArtifactRefs:   datatypes.JSON(`[{"artifact_id":"artifact-1","artifact_type":"report"}]`),
		},
		Run:              &Run{ID: uuid.New()},
		Source:           MemoryExtractionSource{Query: "review launch", Answer: "launch is September 8"},
		AuthorizedScopes: []memoryruntime.MemoryScopeRef{{Type: memoryruntime.MemoryScopeProject, ID: "project-7"}},
		CurrentMemories: []*DigitalHumanMemory{{
			ID: memoryID, MemoryKey: "launch-v1", MemoryClass: memoryruntime.MemoryClassBusiness,
			MemoryType: memoryruntime.MemoryTypeFact, ScopeType: memoryruntime.MemoryScopeProject,
			ScopeID: "project-7", Content: "launch is September 8", Revision: 4,
		}},
	}
	prompt, err := memoryExtractionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"artifact-1", "project-7", "launch is September 8", memoryID.String(), "supersedes_id", "self/decision", "published identity"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
	if strings.Contains(prompt, "artifact bodies") && !strings.Contains(prompt, "Never propose") {
		t.Fatal("prompt does not retain the artifact-body prohibition")
	}
}

func TestMemoryPromotionRequiresTrustedEventFlag(t *testing.T) {
	t.Parallel()
	event := invocation.Event{Context: map[string]any{"memory_promotion_authorized": "true"}}
	if memoryPromotionAuthorized(event) {
		t.Fatal("string promotion flag was trusted")
	}
	event.Context["memory_promotion_authorized"] = true
	if !memoryPromotionAuthorized(event) {
		t.Fatal("trusted boolean promotion flag was ignored")
	}
}

func TestMemoryScopeAuthorizationRequiresExactTypeAndID(t *testing.T) {
	t.Parallel()
	authorized := []memoryruntime.MemoryScopeRef{{Type: memoryruntime.MemoryScopeProject, ID: "project-7"}}
	if !memoryScopeAuthorized(memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeProject, ID: "project-7"}, authorized) {
		t.Fatal("exact scope was rejected")
	}
	if memoryScopeAuthorized(memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeCustomer, ID: "project-7"}, authorized) {
		t.Fatal("different scope type was accepted")
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID { return &value }
