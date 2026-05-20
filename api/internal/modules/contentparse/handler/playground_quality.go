package handler

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func buildPlaygroundQualitySummary(artifact *contracts.ParseArtifact, duration time.Duration) playgroundQualitySummary {
	if artifact == nil {
		return playgroundQualitySummary{
			Status:       contracts.ParseStatusFailed,
			QualityLevel: contracts.ParseQualityFailed,
			DurationMS:   duration.Milliseconds(),
		}
	}

	bboxCount := 0
	reliableBBox := 0
	unreliableBBox := 0
	confidenceCount := 0
	confidenceTotal := 0.0
	pageCount := 0
	for _, element := range artifact.Elements {
		if element.BBox != nil {
			bboxCount++
			if strings.EqualFold(element.Precision, "unreliable") {
				unreliableBBox++
			} else {
				reliableBBox++
			}
		}
		if element.Confidence != nil {
			confidenceCount++
			confidenceTotal += *element.Confidence
		}
		if element.Page+1 > pageCount {
			pageCount = element.Page + 1
		}
	}

	summary := playgroundQualitySummary{
		Status:         artifact.Status,
		QualityLevel:   artifact.QualityLevel,
		EngineUsed:     artifact.EngineUsed,
		FallbackUsed:   artifact.FallbackUsed,
		DurationMS:     duration.Milliseconds(),
		TextLength:     len([]rune(artifact.Text)),
		MarkdownLength: len([]rune(artifact.Markdown)),
		ElementCount:   len(artifact.Elements),
		BBoxCount:      bboxCount,
		ReliableBBox:   reliableBBox,
		UnreliableBBox: unreliableBBox,
		PageCount:      pageCount,
	}
	if len(artifact.Elements) > 0 {
		summary.BBoxRatio = roundFloat(float64(bboxCount)/float64(len(artifact.Elements)), 4)
	}
	if bboxCount > 0 {
		summary.ReliableRatio = roundFloat(float64(reliableBBox)/float64(bboxCount), 4)
	}
	if confidenceCount > 0 {
		summary.AvgConfidence = roundFloat(confidenceTotal/float64(confidenceCount), 4)
	}
	summary.OCREngine = stringFromDiagnostics(artifact.Diagnostics, "ocr_engine")
	summary.OCRStrategy = stringFromDiagnostics(artifact.Diagnostics, "ocr_strategy")
	return summary
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func stringFromDiagnostics(diagnostics map[string]any, key string) string {
	if len(diagnostics) == 0 {
		return ""
	}
	value, ok := diagnostics[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
