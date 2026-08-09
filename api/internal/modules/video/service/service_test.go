package service

import "testing"

func TestEstimateVideoTaskCreditsUsesDefaultPerSecondPoints(t *testing.T) {
	got := estimateVideoTaskCredits(GenerateOptions{Duration: 4, Count: 1})
	const want int64 = 572000
	if got != want {
		t.Fatalf("estimateVideoTaskCredits() = %d, want %d", got, want)
	}
}

func TestEstimateVideoTaskCreditsFallsBackToDefaults(t *testing.T) {
	got := estimateVideoTaskCredits(GenerateOptions{})
	const want int64 = 715000
	if got != want {
		t.Fatalf("estimateVideoTaskCredits() = %d, want %d", got, want)
	}
}
