package digitalhuman

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/datatypes"
)

func TestParseMemoryExtractionDraftsNormalizesStableKeys(t *testing.T) {
	t.Parallel()
	targetID := uuid.New()
	drafts, err := parseMemoryExtractionDrafts(`{"proposals":[{"key":" Launch-Date ","memory_type":"fact","content":"Launch is September 8.","confidence":0.9,"supersedes_id":"` + targetID.String() + `","expected_superseded_revision":3}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].Key != "launch-date" || drafts[0].Confidence != 0.9 ||
		drafts[0].SupersedesID == nil || *drafts[0].SupersedesID != targetID || drafts[0].ExpectedSupersededRevision != 3 {
		t.Fatalf("drafts = %#v", drafts)
	}
}

func TestParseMemoryExtractionDraftsRejectsDuplicateIdentity(t *testing.T) {
	t.Parallel()
	_, err := parseMemoryExtractionDrafts(`{"proposals":[
		{"key":"launch","memory_type":"decision","content":"A"},
		{"key":"launch","memory_type":"decision","content":"B"}
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
		Run:    &Run{ID: uuid.New()},
		Source: MemoryExtractionSource{Query: "review launch", Answer: "launch is September 8"},
		CurrentMemories: []*DigitalHumanMemory{{
			ID: memoryID, MemoryKey: "launch-v1", MemoryClass: memoryruntime.MemoryClassSelf,
			MemoryType: memoryruntime.MemoryTypeFact, Content: "launch is September 8", Revision: 4,
		}},
	}
	prompt, err := memoryExtractionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"artifact-1", "launch is September 8", memoryID.String(), "supersedes_id", "as a decision", "published identity", "All memory is self memory"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
	for _, unexpected := range []string{"scope_type", "scope_id", "memory_class", "authorized_business_scopes"} {
		if strings.Contains(prompt, unexpected) {
			t.Errorf("prompt unexpectedly exposes converged scope field %q", unexpected)
		}
	}
	if strings.Contains(prompt, "artifact bodies") && !strings.Contains(prompt, "Never propose") {
		t.Fatal("prompt does not retain the artifact-body prohibition")
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID { return &value }
