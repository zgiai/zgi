package quality

type ScoreInput struct {
	UnitCount            int
	TotalChars           int
	AvgChars             float64
	BBoxCoverage         float64
	PageCoverage         int
	StableOrder          bool
	LowValueRemovedCount int
	LegacyUnitCount      int
	LegacyTotalChars     int
}

type Score struct {
	Overall  int                `json:"overall"`
	Label    string             `json:"label"`
	Signals  map[string]float64 `json:"signals,omitempty"`
	Warnings []string           `json:"warnings,omitempty"`
}

func EvaluateChunkScore(input ScoreInput) Score {
	signals := map[string]float64{
		"avg_chars":              input.AvgChars,
		"bbox_coverage":          clampRatio(input.BBoxCoverage),
		"low_value_filter_ratio": lowValueFilterRatio(input.LowValueRemovedCount, input.UnitCount),
	}
	if input.LegacyUnitCount > 0 {
		signals["compaction_ratio"] = ratioFloat(input.UnitCount, input.LegacyUnitCount)
	}
	if input.LegacyTotalChars > 0 {
		signals["text_retention_ratio"] = ratioFloat(input.TotalChars, input.LegacyTotalChars)
	}

	if input.UnitCount <= 0 || input.TotalChars <= 0 {
		return Score{
			Overall: 0,
			Label:   "failed",
			Signals: signals,
			Warnings: []string{
				"empty_chunk_output",
			},
		}
	}

	score := 100
	warnings := make([]string, 0)
	if !input.StableOrder {
		score -= 20
		warnings = append(warnings, "unstable_order")
	}
	if input.AvgChars > 0 && input.AvgChars < 80 {
		score -= 15
		warnings = append(warnings, "short_average_chunk")
	}
	if input.AvgChars > 4000 {
		score -= 10
		warnings = append(warnings, "oversized_average_chunk")
	}
	if input.PageCoverage > 0 && input.BBoxCoverage > 0 && input.BBoxCoverage < 0.2 {
		score -= 8
		warnings = append(warnings, "low_bbox_coverage")
	}

	filterRatio := signals["low_value_filter_ratio"]
	if filterRatio > 0.6 {
		score -= 12
		warnings = append(warnings, "very_noisy_source")
	} else if filterRatio > 0.35 {
		score -= 8
		warnings = append(warnings, "noisy_source")
	}

	if input.LegacyTotalChars > 0 {
		retention := signals["text_retention_ratio"]
		if retention < 0.55 {
			score -= 35
			warnings = append(warnings, "large_text_loss")
		} else if retention < 0.8 {
			score -= 10
			warnings = append(warnings, "text_loss")
		}
		if retention > 1.35 {
			score -= 8
			warnings = append(warnings, "large_text_expansion")
		}
	}
	if input.LegacyUnitCount > 0 {
		compaction := signals["compaction_ratio"]
		if compaction > 2.0 {
			score -= 10
			warnings = append(warnings, "chunk_count_expansion")
		}
	}

	score = clampScore(score)
	return Score{
		Overall:  score,
		Label:    scoreLabel(score),
		Signals:  signals,
		Warnings: warnings,
	}
}

func scoreLabel(score int) string {
	switch {
	case score >= 85:
		return "high"
	case score >= 70:
		return "standard"
	case score >= 50:
		return "degraded"
	default:
		return "failed"
	}
}

func lowValueFilterRatio(removed, kept int) float64 {
	total := removed + kept
	if total <= 0 {
		return 0
	}
	return float64(removed) / float64(total)
}

func ratioFloat(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
