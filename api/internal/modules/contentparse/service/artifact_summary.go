package service

import (
	"sort"

	"github.com/zgiai/zgi/api/internal/contracts"
)

// ParseArtifactStorageSummary keeps parse artifact persistence metadata stable
// while the dataset indexing caller is gradually moved out of the artifact layer.
func ParseArtifactStorageSummary(artifact *contracts.ParseArtifact) map[string]interface{} {
	if artifact == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"artifact_id":     artifact.ArtifactID,
		"status":          artifact.Status,
		"quality_level":   artifact.QualityLevel,
		"engine_used":     artifact.EngineUsed,
		"fallback_used":   artifact.FallbackUsed,
		"text_length":     len(artifact.Text),
		"markdown_length": len(artifact.Markdown),
		"element_count":   len(artifact.Elements),
		"diagnostics":     ParseArtifactDiagnosticsSummary(artifact.Diagnostics),
	}
}

func ParseArtifactDiagnosticsSummary(diag map[string]any) map[string]interface{} {
	if len(diag) == 0 {
		return nil
	}
	keys := make([]string, 0, len(diag))
	for key := range diag {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	summary := map[string]interface{}{
		"keys": keys,
	}
	if value, ok := diag["recognition_source"]; ok {
		summary["recognition_source"] = value
	}
	if value, ok := diag["local_vlm_fallback"]; ok {
		summary["local_vlm_fallback"] = value
	}
	if value, ok := diag["ocr_fallback"]; ok {
		summary["ocr_fallback"] = value
	}
	if value, ok := diag["ocr_engine"]; ok {
		summary["ocr_engine"] = value
	}
	if value, ok := diag["ocr_strategy"]; ok {
		summary["ocr_strategy"] = value
	}
	if value, ok := diag["ocr_retry_used"]; ok {
		summary["ocr_retry_used"] = value
	}
	if value, ok := diag["ocr_preprocess"]; ok {
		summary["ocr_preprocess"] = value
	}
	if value, ok := diag["local_image_parse"]; ok {
		summary["local_image_parse"] = value
	}
	return summary
}
