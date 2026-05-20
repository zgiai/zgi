package pdf

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	pdfadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
)

var (
	pageAmountLikeRE   = regexp.MustCompile(`(?:[$¥￥]\s*)?\d{1,3}(?:,\d{3})*(?:\.\d{2})?`)
	pageDateLikeRE     = regexp.MustCompile("(?:\\b\\d{4}[-/]\\d{1,2}[-/]\\d{1,2}\\b|\\b\\d{1,2}[-/]\\d{1,2}[-/]\\d{2,4}\\b|\\d{4}\u5e74\\d{1,2}\u6708\\d{1,2}\u65e5)")
	pageAccountLikeRE  = regexp.MustCompile(`(?:\b\d{8,}\b|\b(?:account|acct|policy|member|claim|invoice|statement|id)\b[\s:#-]*[A-Za-z0-9-]{4,})`)
	pageCheckboxLikeRE = regexp.MustCompile(`[☑☐□✓✔]`)
)

type pageRouteStats struct {
	pageIndex             int
	geometryLineCount     int
	geometryTokenCount    int
	lineRuneCount         int
	tokenRuneCount        int
	shortLineCount        int
	imageCount            int
	kvLineCount           int
	amountLikeLineCount   int
	dateLikeLineCount     int
	accountLikeLineCount  int
	checkboxLikeLineCount int
	leftAnchors           []float64
}

func buildPageRouteCandidates(
	totalPages int,
	geometryLines []pdfadapter.GeometryLine,
	geometryTokens []pdfadapter.GeometryToken,
	images []pdfadapter.ExtractedImageBytes,
	docBusinessHint pdfadapter.BusinessDocVLMRouteHint,
	force bool,
) []map[string]any {
	if totalPages <= 0 {
		return nil
	}

	stats := make([]pageRouteStats, totalPages)
	for i := range stats {
		stats[i].pageIndex = i + 1
	}

	for _, line := range geometryLines {
		pageIdx := line.PageIndex
		if pageIdx < 1 || pageIdx > totalPages {
			continue
		}
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		st := &stats[pageIdx-1]
		st.geometryLineCount++
		runes := utf8.RuneCountInString(text)
		st.lineRuneCount += runes
		if runes <= 10 {
			st.shortLineCount++
		}
		if strings.Contains(text, ":") || strings.Contains(text, "：") {
			st.kvLineCount++
		}
		if pageAmountLikeRE.MatchString(text) {
			st.amountLikeLineCount++
		}
		if pageDateLikeRE.MatchString(text) {
			st.dateLikeLineCount++
		}
		if pageAccountLikeRE.MatchString(strings.ToLower(text)) {
			st.accountLikeLineCount++
		}
		if pageCheckboxLikeRE.MatchString(text) {
			st.checkboxLikeLineCount++
		}
		if line.GeomX != 0 {
			st.leftAnchors = append(st.leftAnchors, line.GeomX)
		}
	}

	for _, token := range geometryTokens {
		pageIdx := token.PageIndex
		if pageIdx < 1 || pageIdx > totalPages {
			continue
		}
		text := strings.TrimSpace(token.Text)
		if text == "" {
			continue
		}
		st := &stats[pageIdx-1]
		st.geometryTokenCount++
		st.tokenRuneCount += utf8.RuneCountInString(text)
		if st.geometryLineCount == 0 && token.GeomX != 0 {
			st.leftAnchors = append(st.leftAnchors, token.GeomX)
		}
	}

	for _, image := range images {
		pageIdx := image.PageIndex
		if pageIdx < 1 || pageIdx > totalPages {
			continue
		}
		stats[pageIdx-1].imageCount++
	}

	candidates := make([]map[string]any, 0, totalPages)
	for _, st := range stats {
		candidates = append(candidates, st.buildCandidate(docBusinessHint, force))
	}
	return candidates
}

func (st pageRouteStats) buildCandidate(docBusinessHint pdfadapter.BusinessDocVLMRouteHint, force bool) map[string]any {
	reasons := make([]map[string]any, 0, 4)
	addReason := func(code string, score float64, detail string) {
		reason := map[string]any{
			"code":  code,
			"score": roundRouteScore(score),
		}
		if strings.TrimSpace(detail) != "" {
			reason["detail"] = strings.TrimSpace(detail)
		}
		reasons = append(reasons, reason)
	}

	if force {
		addReason("force_vlm", 1.0, envForceVLM)
	}

	textRunes := st.textRuneCount()
	leftClusters := st.leftClusterCount()
	shortLineRatio := st.shortLineRatio()
	avgLineRunes := st.avgLineRunes()

	if st.imageCount > 0 && (textRunes <= 24 || st.geometryTokenCount <= 6) {
		score := 0.74
		detail := "thin_text_with_embedded_images"
		if textRunes <= 12 {
			score = 0.84
			detail = "minimal_text_with_embedded_images"
		}
		addReason("scan_like", score, detail)
	}

	formEvidence := 0
	if st.kvLineCount >= 3 {
		formEvidence++
	}
	if st.amountLikeLineCount >= 2 {
		formEvidence++
	}
	if st.dateLikeLineCount >= 1 {
		formEvidence++
	}
	if st.accountLikeLineCount >= 1 {
		formEvidence++
	}
	if st.checkboxLikeLineCount >= 2 {
		formEvidence++
	}
	if formEvidence >= 3 {
		score := 0.62 + 0.05*float64(formEvidence)
		if score > 0.82 {
			score = 0.82
		}
		addReason("business_form_like", score, st.businessFormDetail())
	} else if st.shouldEscalateBusinessFormFromDocHint(docBusinessHint, formEvidence) {
		score := 0.64 + 0.03*float64(formEvidence)
		if st.amountLikeLineCount >= 6 {
			score += 0.05
		}
		if st.imageCount > 0 {
			score += 0.03
		}
		if score > 0.79 {
			score = 0.79
		}
		addReason("business_form_like", score, st.businessFormDocHintDetail())
	}

	if st.geometryLineCount >= 8 && shortLineRatio >= 0.6 && leftClusters >= 3 {
		score := 0.68 + 0.03*float64(leftClusters-3)
		if score > 0.8 {
			score = 0.8
		}
		addReason("native_quality_low", score, "fragmented_multi_column_text")
	} else if st.geometryTokenCount >= 18 && avgLineRunes > 0 && avgLineRunes <= 6.5 && leftClusters >= 2 {
		addReason("native_quality_low", 0.66, "dense_short_lines")
	}

	recommendedMode := "native_only"
	if force {
		recommendedMode = "force_vlm"
	} else if len(reasons) > 0 {
		recommendedMode = "vlm_candidate"
	}

	scores := make([]float64, 0, len(reasons))
	for _, reason := range reasons {
		if score, ok := reason["score"].(float64); ok {
			scores = append(scores, score)
		}
	}

	return map[string]any{
		"page_index":       st.pageIndex,
		"recommended_mode": recommendedMode,
		"score":            roundRouteScore(combineRouteScores(scores)),
		"reasons":          reasons,
		"selected_for_vlm": recommendedMode != "native_only",
		"native_signals": map[string]any{
			"geometry_line_count":      st.geometryLineCount,
			"geometry_token_count":     st.geometryTokenCount,
			"text_rune_count":          textRunes,
			"image_count":              st.imageCount,
			"short_line_ratio":         roundRouteScore(shortLineRatio),
			"avg_line_runes":           roundRouteScore(avgLineRunes),
			"left_cluster_count":       leftClusters,
			"kv_line_count":            st.kvLineCount,
			"amount_like_line_count":   st.amountLikeLineCount,
			"date_like_line_count":     st.dateLikeLineCount,
			"account_like_line_count":  st.accountLikeLineCount,
			"checkbox_like_line_count": st.checkboxLikeLineCount,
		},
	}
}

func (st pageRouteStats) shouldEscalateBusinessFormFromDocHint(docBusinessHint pdfadapter.BusinessDocVLMRouteHint, formEvidence int) bool {
	if !docBusinessHint.Suggest {
		return false
	}
	if formEvidence < 2 {
		return false
	}
	if st.amountLikeLineCount < 2 {
		return false
	}
	if st.geometryLineCount < 8 && st.geometryTokenCount < 30 {
		return false
	}
	return st.imageCount > 0 || st.amountLikeLineCount >= 4 || st.kvLineCount >= 2
}

func (st pageRouteStats) textRuneCount() int {
	if st.lineRuneCount > 0 {
		return st.lineRuneCount
	}
	return st.tokenRuneCount
}

func (st pageRouteStats) shortLineRatio() float64 {
	if st.geometryLineCount == 0 {
		return 0
	}
	return float64(st.shortLineCount) / float64(st.geometryLineCount)
}

func (st pageRouteStats) avgLineRunes() float64 {
	if st.geometryLineCount == 0 {
		return 0
	}
	return float64(st.lineRuneCount) / float64(st.geometryLineCount)
}

func (st pageRouteStats) leftClusterCount() int {
	if len(st.leftAnchors) == 0 {
		return 0
	}
	xs := append([]float64(nil), st.leftAnchors...)
	sort.Float64s(xs)
	clusters := 1
	last := xs[0]
	for _, x := range xs[1:] {
		if x-last > 28 {
			clusters++
		}
		last = x
	}
	return clusters
}

func (st pageRouteStats) businessFormDetail() string {
	parts := make([]string, 0, 5)
	if st.kvLineCount >= 3 {
		parts = append(parts, fmt.Sprintf("kv=%d", st.kvLineCount))
	}
	if st.amountLikeLineCount >= 2 {
		parts = append(parts, fmt.Sprintf("amount=%d", st.amountLikeLineCount))
	}
	if st.dateLikeLineCount >= 1 {
		parts = append(parts, fmt.Sprintf("date=%d", st.dateLikeLineCount))
	}
	if st.accountLikeLineCount >= 1 {
		parts = append(parts, fmt.Sprintf("account=%d", st.accountLikeLineCount))
	}
	if st.checkboxLikeLineCount >= 2 {
		parts = append(parts, fmt.Sprintf("checkbox=%d", st.checkboxLikeLineCount))
	}
	return strings.Join(parts, ",")
}

func (st pageRouteStats) businessFormDocHintDetail() string {
	parts := make([]string, 0, 6)
	parts = append(parts, "doc_hint=true")
	if form := st.businessFormDetail(); form != "" {
		parts = append(parts, form)
	}
	if st.imageCount > 0 {
		parts = append(parts, fmt.Sprintf("image=%d", st.imageCount))
	}
	if st.geometryLineCount >= 8 {
		parts = append(parts, fmt.Sprintf("geom_lines=%d", st.geometryLineCount))
	}
	return strings.Join(parts, ",")
}
