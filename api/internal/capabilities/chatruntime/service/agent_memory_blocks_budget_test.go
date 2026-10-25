package service

import "testing"

func TestMemoryBlocksBudgetHoldsEveryFullBlock(t *testing.T) {
	slots := []AgentMemorySlotConfig{{Key: "a", MaxChars: 2000}, {Key: "b", MaxChars: 2000}, {Key: "c", MaxChars: 2000}, {Key: "d", MaxChars: 2000, Description: "Ongoing work."}}
	if budget := memoryBlocksContextChars(slots); budget < 8000+4*100 {
		t.Fatalf("budget %d cannot hold four full blocks and their framing", budget)
	}
	if budget := memoryBlocksContextChars(slots[:1]); budget != minMemoryBlocksContextChars {
		t.Fatalf("one block budget = %d, want the %d floor", budget, minMemoryBlocksContextChars)
	}
}
