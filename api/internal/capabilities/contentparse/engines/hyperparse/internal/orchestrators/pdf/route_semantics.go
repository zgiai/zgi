package pdf

import (
	"math"
	"sort"
	"strconv"
	"strings"

	pdfadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
)

const envForceVLM = "CONTENT_PARSE_FORCE_VLM"

func forceVLMFromEnv() bool {
	return envTruthy(contentParseEnv(envForceVLM))
}

func buildRouteDecision(
	imgHints pdfadapter.ImageLikePDFHints,
	bizVLM pdfadapter.BusinessDocVLMRouteHint,
	pageCandidates []map[string]any,
) map[string]any {
	return buildRouteDecisionWithForce(imgHints, bizVLM, pageCandidates, forceVLMFromEnv())
}

func buildRouteDecisionWithForce(
	imgHints pdfadapter.ImageLikePDFHints,
	bizVLM pdfadapter.BusinessDocVLMRouteHint,
	pageCandidates []map[string]any,
	force bool,
) map[string]any {
	reasons := make([]map[string]any, 0, 3)
	reasonIndex := make(map[string]int, 4)
	addReason := func(code string, score float64, detail string) {
		score = roundRouteScore(score)
		detail = strings.TrimSpace(detail)
		if idx, ok := reasonIndex[code]; ok {
			if prev, ok := reasons[idx]["score"].(float64); !ok || score > prev {
				reasons[idx]["score"] = score
			}
			if detail == "" {
				return
			}
			prevDetail, _ := reasons[idx]["detail"].(string)
			switch {
			case prevDetail == "":
				reasons[idx]["detail"] = detail
			case !strings.Contains(prevDetail, detail):
				reasons[idx]["detail"] = prevDetail + ";" + detail
			}
			return
		}
		reason := map[string]any{
			"code":  code,
			"score": score,
		}
		if detail != "" {
			reason["detail"] = detail
		}
		reasonIndex[code] = len(reasons)
		reasons = append(reasons, reason)
	}

	if imgHints.Likely {
		score := 0.62 + 0.08*float64(len(imgHints.Reasons))
		if score > 0.85 {
			score = 0.85
		}
		addReason("scan_like", score, strings.Join(imgHints.Reasons, ","))
	}

	if bizVLM.Suggest {
		evidenceCount := len(bizVLM.Kinds)
		if len(bizVLM.Reasons) > evidenceCount {
			evidenceCount = len(bizVLM.Reasons)
		}
		score := 0.55 + 0.06*float64(evidenceCount)
		if score > 0.78 {
			score = 0.78
		}
		detail := strings.Join(bizVLM.Kinds, ",")
		if detail == "" {
			detail = strings.Join(bizVLM.Reasons, ",")
		}
		addReason("business_form_like", score, detail)
	}

	if force {
		addReason("force_vlm", 1.0, envForceVLM)
	}
	addPageCandidateReasons(addReason, pageCandidates)

	recommendedMode := "native_only"
	switch {
	case force:
		recommendedMode = "force_vlm"
	case len(reasons) > 0:
		recommendedMode = "vlm_candidate"
	}

	scores := make([]float64, 0, len(reasons))
	for _, reason := range reasons {
		if score, ok := reason["score"].(float64); ok {
			scores = append(scores, score)
		}
	}

	return map[string]any{
		"version":            "v1",
		"recommended_mode":   recommendedMode,
		"score":              roundRouteScore(combineRouteScores(scores)),
		"reasons":            reasons,
		"legacy_suggest_vlm": force || len(reasons) > 0,
	}
}

func addPageCandidateReasons(addReason func(code string, score float64, detail string), pageCandidates []map[string]any) {
	if len(pageCandidates) == 0 {
		return
	}
	pageNumbersByCode := map[string][]int{}
	maxScoreByCode := map[string]float64{}
	for _, candidate := range pageCandidates {
		pageIndex, _ := candidate["page_index"].(int)
		reasons, _ := candidate["reasons"].([]map[string]any)
		for _, reason := range reasons {
			code, _ := reason["code"].(string)
			if code == "" || code == "force_vlm" {
				continue
			}
			pageNumbersByCode[code] = append(pageNumbersByCode[code], pageIndex)
			if score, ok := reason["score"].(float64); ok && score > maxScoreByCode[code] {
				maxScoreByCode[code] = score
			}
		}
	}
	for _, code := range []string{"scan_like", "business_form_like", "native_quality_low"} {
		pages := uniqueSortedPageNumbers(pageNumbersByCode[code])
		if len(pages) == 0 {
			continue
		}
		addReason(code, maxScoreByCode[code], "pages="+joinPageNumbers(pages))
	}
}

func uniqueSortedPageNumbers(pages []int) []int {
	if len(pages) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(pages))
	out := make([]int, 0, len(pages))
	for _, page := range pages {
		if page < 1 || seen[page] {
			continue
		}
		seen[page] = true
		out = append(out, page)
	}
	sort.Ints(out)
	return out
}

func joinPageNumbers(pages []int) string {
	if len(pages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pages))
	for _, page := range pages {
		parts = append(parts, strconv.Itoa(page))
	}
	return strings.Join(parts, ",")
}

func combineRouteScores(scores []float64) float64 {
	combined := 0.0
	for _, score := range scores {
		if score <= 0 {
			continue
		}
		if score > 1 {
			score = 1
		}
		combined = 1 - (1-combined)*(1-score)
	}
	return combined
}

func roundRouteScore(score float64) float64 {
	if score <= 0 {
		return 0
	}
	if score >= 1 {
		return 1
	}
	return math.Round(score*1000) / 1000
}
