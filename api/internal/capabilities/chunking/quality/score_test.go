package quality

import "testing"

func TestEvaluateChunkScoreHighQuality(t *testing.T) {
	score := EvaluateChunkScore(ScoreInput{
		UnitCount:            5,
		TotalChars:           2400,
		AvgChars:             480,
		BBoxCoverage:         0.8,
		PageCoverage:         3,
		StableOrder:          true,
		LowValueRemovedCount: 1,
		LegacyUnitCount:      8,
		LegacyTotalChars:     2500,
	})
	if score.Label != "high" {
		t.Fatalf("label=%q score=%d warnings=%v", score.Label, score.Overall, score.Warnings)
	}
	if score.Signals["text_retention_ratio"] < 0.9 {
		t.Fatalf("retention=%v", score.Signals["text_retention_ratio"])
	}
}

func TestEvaluateChunkScoreDetectsTextLoss(t *testing.T) {
	score := EvaluateChunkScore(ScoreInput{
		UnitCount:        2,
		TotalChars:       400,
		AvgChars:         200,
		StableOrder:      true,
		LegacyUnitCount:  4,
		LegacyTotalChars: 2000,
	})
	if score.Overall >= 70 {
		t.Fatalf("score=%d warnings=%v", score.Overall, score.Warnings)
	}
	if !containsWarning(score.Warnings, "large_text_loss") {
		t.Fatalf("warnings=%v", score.Warnings)
	}
}

func TestEvaluateChunkScoreEmptyOutputFails(t *testing.T) {
	score := EvaluateChunkScore(ScoreInput{})
	if score.Overall != 0 || score.Label != "failed" {
		t.Fatalf("score=%d label=%q", score.Overall, score.Label)
	}
}

func containsWarning(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
