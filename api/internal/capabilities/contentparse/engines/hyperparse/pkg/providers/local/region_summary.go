package local

import (
	"strings"

	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

type localRegionSummary struct {
	Pages             int            `json:"pages"`
	Chunks            int            `json:"chunks"`
	Regions           int            `json:"regions"`
	Tables            int            `json:"tables"`
	Figures           int            `json:"figures"`
	Sidebar           int            `json:"sidebar"`
	Header            int            `json:"header"`
	Footer            int            `json:"footer"`
	OCRChunks         int            `json:"ocr_chunks"`
	VLMChunks         int            `json:"vlm_chunks"`
	RuleChunks        int            `json:"rule_chunks"`
	ReliableBBox      int            `json:"reliable_bbox"`
	CoarseBBox        int            `json:"coarse_bbox"`
	UnreliableBBox    int            `json:"unreliable_bbox"`
	ByType            map[string]int `json:"by_type,omitempty"`
	ExtractionMethods map[string]int `json:"extraction_methods,omitempty"`
}

func attachLocalRegionSummary(doc *extractcommon.DocumentResult) {
	if doc == nil {
		return
	}
	if doc.Diagnostics == nil {
		doc.Diagnostics = map[string]any{}
	}
	summary := summarizeLocalRegions(doc)
	doc.Diagnostics["region_summary"] = localRegionSummaryAsMap(summary)
}

func summarizeLocalRegions(doc *extractcommon.DocumentResult) localRegionSummary {
	summary := localRegionSummary{
		Pages:             doc.PageCount,
		Chunks:            len(doc.Chunks),
		ByType:            map[string]int{},
		ExtractionMethods: map[string]int{},
	}
	if summary.Pages <= 0 {
		summary.Pages = len(doc.Pages)
	}

	for _, ch := range doc.Chunks {
		typ := strings.ToLower(strings.TrimSpace(ch.Type))
		if typ == "" {
			typ = "text"
		}
		summary.ByType[typ]++
		switch typ {
		case "table":
			summary.Tables++
		case "figure", "image", "stamp", "logo", "scan_code":
			summary.Figures++
		}

		if isHeaderChunk(ch) {
			summary.Header++
		}
		if isFooterChunk(ch) {
			summary.Footer++
		}
		if typ == "marginalia" || isSidebarChunk(ch) {
			summary.Sidebar++
		}

		method := inferChunkMethod(ch)
		if method == "" {
			method = "rule"
		}
		summary.ExtractionMethods[method]++
		switch {
		case strings.Contains(method, "vlm"):
			summary.VLMChunks++
		case strings.Contains(method, "ocr"):
			summary.OCRChunks++
		default:
			summary.RuleChunks++
		}

		switch strings.ToLower(strings.TrimSpace(ch.Precision)) {
		case "unreliable":
			summary.UnreliableBBox++
		case "coarse":
			summary.CoarseBBox++
		default:
			if ch.BBox != nil {
				summary.ReliableBBox++
			} else {
				summary.UnreliableBBox++
			}
		}
	}

	summary.Regions = summary.Tables + summary.Figures + summary.Sidebar + summary.Header + summary.Footer
	if summary.Regions == 0 && summary.Chunks > 0 {
		summary.Regions = summary.Chunks
	}
	return summary
}

func inferChunkMethod(ch extractcommon.Chunk) string {
	if ch.Payload != nil {
		if v := payloadString(ch.Payload, "extraction_method"); v != "" {
			return strings.ToLower(v)
		}
		if v := payloadString(ch.Payload, "vlm_merge"); strings.HasPrefix(strings.ToLower(v), "from_vlm") {
			return "vlm"
		}
		if v := payloadString(ch.Payload, "extraction_source_raw"); v != "" {
			v = strings.ToLower(v)
			if v == "vlm" || v == "ocr" {
				return v
			}
		}
		if payloadString(ch.Payload, "vlm_caption") != "" || payloadString(ch.Payload, "vlm_caption_source") != "" {
			return "vlm_caption"
		}
		if payloadString(ch.Payload, "sidebar_recovery_engine") != "" {
			return "ocr"
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ch.ID)), "local-ocr-") {
		return "ocr"
	}
	return ""
}

func isSidebarChunk(ch extractcommon.Chunk) bool {
	if ch.Payload == nil {
		return false
	}
	if v, ok := ch.Payload["sidebar_recovery"].(bool); ok && v {
		return true
	}
	return payloadString(ch.Payload, "sidebar_recovery_region") != ""
}

func isHeaderChunk(ch extractcommon.Chunk) bool {
	if ch.BBox == nil {
		return false
	}
	return ch.BBox.Bottom > 0 && ch.BBox.Bottom <= 0.16
}

func isFooterChunk(ch extractcommon.Chunk) bool {
	if ch.BBox == nil {
		return false
	}
	return ch.BBox.Top >= 0.84
}

func localRegionSummaryAsMap(summary localRegionSummary) map[string]any {
	out := map[string]any{
		"pages":              summary.Pages,
		"chunks":             summary.Chunks,
		"regions":            summary.Regions,
		"tables":             summary.Tables,
		"figures":            summary.Figures,
		"sidebar":            summary.Sidebar,
		"header":             summary.Header,
		"footer":             summary.Footer,
		"ocr_chunks":         summary.OCRChunks,
		"vlm_chunks":         summary.VLMChunks,
		"rule_chunks":        summary.RuleChunks,
		"reliable_bbox":      summary.ReliableBBox,
		"coarse_bbox":        summary.CoarseBBox,
		"unreliable_bbox":    summary.UnreliableBBox,
		"by_type":            summary.ByType,
		"extraction_methods": summary.ExtractionMethods,
	}
	return out
}
