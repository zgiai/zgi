package agentmemoryworker

import (
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func TestParseAutomaticOperationsAcceptsOnlyUpsertOrNone(t *testing.T) {
	operations, err := parseAutomaticOperations(`{"operations":[{"action":"upsert","key":"profile","content":"Prefers concise replies","evidence":"I prefer concise replies","confidence":0.42},{"action":"none","key":"project"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || operations[0].Action != "upsert" || operations[1].Action != "none" {
		t.Fatalf("operations = %#v", operations)
	}
	if _, err := parseAutomaticOperations(`{"operations":[{"action":"clear","key":"profile"}]}`); err == nil {
		t.Fatal("clear operation unexpectedly accepted")
	}
	if _, err := parseAutomaticOperations(`{"operations":[{"action":"upsert","key":"profile","content":"one"},{"action":"upsert","key":"profile","content":"two"}]}`); err == nil {
		t.Fatal("duplicate slot operations unexpectedly accepted")
	}
}

func TestAutomaticOperationRequiresEnabledSlotAndExactEvidence(t *testing.T) {
	slots := []agentmemory.RuntimeSlot{
		{Key: "profile", Enabled: true},
		{Key: "standing_instructions", Enabled: true},
	}
	if _, ok := automaticSlot(slots, "profile"); !ok {
		t.Fatal("automatic profile slot not found")
	}
	if _, ok := automaticSlot(slots, "standing_instructions"); !ok {
		t.Fatal("enabled standing instructions slot not found")
	}
	turns := []completedTurn{{Query: "I prefer concise replies."}, {Query: "This is temporary."}}
	if _, ok := evidenceSourceTurn("prefer concise", turns); !ok {
		t.Fatal("exact user evidence was not accepted")
	}
	if _, ok := evidenceSourceTurn("prefers concise", turns); ok {
		t.Fatal("inferred evidence was accepted")
	}
}

func TestFitTurnBudgetKeepsNewestCompleteTurns(t *testing.T) {
	runner := NewRunner(nil, nil, nil)
	turns := []completedTurn{
		{Query: strings.Repeat("old ", 12000), ModelName: "test"},
		{Query: "new durable preference", ModelName: "test"},
	}
	selected, err := runner.fitTurnBudget(turns, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Query != "new durable preference" {
		t.Fatalf("selected turns = %#v", selected)
	}
	request, err := runner.extractionChatRequest(selected, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tokens := runner.estimator.EstimateChatRequest(request).Tokens; tokens > maxExtractionInputTokens {
		t.Fatalf("request tokens = %d, want <= %d", tokens, maxExtractionInputTokens)
	}
}

func TestFitTurnBudgetCountsCurrentMemoryAndSlotPayload(t *testing.T) {
	runner := NewRunner(nil, nil, nil)
	turns := []completedTurn{{Query: "I prefer concise replies", ModelName: "test"}}
	values := []agentmemory.SlotValueResponse{{Content: strings.Repeat("existing memory ", 12000)}}
	selected, err := runner.fitTurnBudget(turns, []agentmemory.RuntimeSlot{{Key: "profile", Enabled: true}}, values)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 0 {
		t.Fatalf("selected turns = %#v, want no request that exceeds the full input budget", selected)
	}
}

func TestExtractionRetryIsBoundedAndBackedOff(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		attemptCount int
		wantDelay    time.Duration
		wantRetry    bool
	}{
		{attemptCount: 1, wantDelay: time.Minute, wantRetry: true},
		{attemptCount: 2, wantDelay: 2 * time.Minute, wantRetry: true},
		{attemptCount: 4, wantDelay: 8 * time.Minute, wantRetry: true},
		{attemptCount: maxExtractionJobAttempts, wantRetry: false},
	}
	for _, test := range tests {
		retryAt, retry := extractionRetryAt(test.attemptCount, now)
		if retry != test.wantRetry {
			t.Fatalf("attempt %d retry = %v, want %v", test.attemptCount, retry, test.wantRetry)
		}
		if retry && retryAt.Sub(now) != test.wantDelay {
			t.Fatalf("attempt %d delay = %v, want %v", test.attemptCount, retryAt.Sub(now), test.wantDelay)
		}
	}
}

func TestGlobalAutomaticExtractionFuseDefaultsClosed(t *testing.T) {
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "")
	if globalAutomaticExtractionEnabled() {
		t.Fatal("automatic extraction fuse unexpectedly enabled by default")
	}
	t.Setenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED", "true")
	if !globalAutomaticExtractionEnabled() {
		t.Fatal("automatic extraction fuse did not enable")
	}
}
