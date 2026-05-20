package inspectsvc

import (
	"sort"

	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

func routeDecisionFromFullDoc(fullDoc map[string]any) any {
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return nil
	}
	return doc["route_decision"]
}

func sequentialProcessedPages(n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func effectiveInspectPageCount(basicPageCount int, fullDoc map[string]any) int {
	if basicPageCount > 0 {
		return basicPageCount
	}
	if fullDoc == nil {
		return 0
	}
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return 0
	}
	layout, ok := doc["layout"].(map[string]any)
	if !ok {
		return 0
	}
	switch pages := layout["pages"].(type) {
	case []any:
		return len(pages)
	case []map[string]any:
		return len(pages)
	}
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return 0
	}
	maxPage := 0
	for _, it := range CoerceChunkItems(chW["items"]) {
		page := IntFromChunkAny(it, "page_index")
		if page > maxPage {
			maxPage = page
		}
	}
	return maxPage
}

func buildInspectSemantics(
	totalPages int,
	plannedPages []int,
	processedPages []int,
	nativeKeptCount int,
	vlmMergedCount int,
	previewPartial bool,
	vlmPagesApplied []int,
) (string, map[string]any, map[string]any) {
	planned := normalizeProcessedPages(plannedPages)
	processed := normalizeProcessedPages(processedPages)
	applied := normalizeProcessedPages(vlmPagesApplied)
	complete := pageSetsEqual(processed, planned)
	if len(planned) == 0 {
		complete = true
	}
	resultScope := "full"
	if previewPartial {
		resultScope = "preview_partial"
	}

	strategy := "native_only"
	if vlmMergedCount > 0 {
		if resultScope == "preview_partial" {
			strategy = "preview_partial_vlm"
		} else {
			strategy = "full_vlm_merge"
		}
	}

	coverage := map[string]any{
		"processed": processed,
		"planned":   planned,
		"total":     totalPages,
		"complete":  complete,
	}
	mergeReport := map[string]any{
		"strategy":          strategy,
		"native_kept_count": nativeKeptCount,
		"vlm_merged_count":  vlmMergedCount,
		"native_pages_kept": complementPages(totalPages, applied),
		"vlm_pages_applied": applied,
		"applied":           vlmMergedCount > 0,
	}
	return resultScope, coverage, mergeReport
}

func normalizeProcessedPages(processedPages []int) []int {
	if len(processedPages) == 0 {
		return []int{}
	}
	seen := make(map[int]bool, len(processedPages))
	out := make([]int, 0, len(processedPages))
	for _, page := range processedPages {
		if page < 1 || seen[page] {
			continue
		}
		seen[page] = true
		out = append(out, page)
	}
	sort.Ints(out)
	return out
}

func pageRouteCandidatePagesFromFullDoc(fullDoc map[string]any) []int {
	candidates := pageRouteCandidatesFromFullDoc(fullDoc)
	if len(candidates) == 0 {
		return nil
	}
	out := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		mode, _ := candidate["recommended_mode"].(string)
		if mode == "" || mode == "native_only" {
			continue
		}
		page, _ := candidate["page_index"].(int)
		if page > 0 {
			out = append(out, page)
		}
	}
	return normalizeProcessedPages(out)
}

func pageRouteCandidatesFromFullDoc(fullDoc map[string]any) []map[string]any {
	doc, ok := fullDoc["document"].(map[string]any)
	if !ok {
		return nil
	}
	rawCandidates, ok := doc["page_route_candidates"]
	if !ok {
		return nil
	}
	candidates, ok := rawCandidates.([]map[string]any)
	if !ok {
		return nil
	}
	return candidates
}

func buildRouteDebug(
	fullDoc map[string]any,
	plannedPages []int,
	processedPages []int,
	appliedPages []int,
) map[string]any {
	planned := normalizeProcessedPages(plannedPages)
	processed := normalizeProcessedPages(processedPages)
	applied := normalizeProcessedPages(appliedPages)
	candidatePages := pageRouteCandidatePagesFromFullDoc(fullDoc)
	candidates := summarizeRouteCandidates(pageRouteCandidatesFromFullDoc(fullDoc), planned, processed, applied)
	debug := map[string]any{
		"candidate_pages":             candidatePages,
		"planned_vlm_pages":           planned,
		"processed_vlm_pages":         processed,
		"applied_vlm_pages":           applied,
		"pending_candidate_pages":     subtractPages(planned, processed),
		"processed_not_applied_pages": subtractPages(processed, applied),
		"candidate_details":           candidates,
	}
	if routeDecision := routeDecisionFromFullDoc(fullDoc); routeDecision != nil {
		debug["document_route_decision"] = routeDecision
	}
	return debug
}

func summarizeRouteCandidates(
	candidates []map[string]any,
	plannedPages []int,
	processedPages []int,
	appliedPages []int,
) []map[string]any {
	if len(candidates) == 0 {
		return []map[string]any{}
	}
	plannedSet := pageMembershipSet(plannedPages)
	processedSet := pageMembershipSet(processedPages)
	appliedSet := pageMembershipSet(appliedPages)
	out := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		page, _ := candidate["page_index"].(int)
		mode, _ := candidate["recommended_mode"].(string)
		if mode == "" {
			continue
		}
		if mode == "native_only" && !plannedSet[page] && !processedSet[page] && !appliedSet[page] {
			continue
		}
		item := map[string]any{
			"page_index":        page,
			"recommended_mode":  mode,
			"selected_for_vlm":  plannedSet[page],
			"processed_for_vlm": processedSet[page],
			"applied_vlm":       appliedSet[page],
		}
		if score, ok := candidate["score"].(float64); ok {
			item["score"] = score
		}
		if reasons, ok := candidate["reasons"].([]map[string]any); ok && len(reasons) > 0 {
			item["reasons"] = reasons
			item["reason_codes"] = candidateReasonCodes(reasons)
		}
		if signals, ok := candidate["native_signals"].(map[string]any); ok && len(signals) > 0 {
			item["native_signals"] = signals
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, _ := out[i]["page_index"].(int)
		pj, _ := out[j]["page_index"].(int)
		return pi < pj
	})
	return out
}

func vlmPageSelectionFromFullDoc(fullDoc map[string]any, totalPages int, maxPages int) []int {
	pages := limitPages(pageRouteCandidatePagesFromFullDoc(fullDoc), maxPages)
	if len(pages) > 0 {
		return pages
	}
	routeDecision, _ := routeDecisionFromFullDoc(fullDoc).(map[string]any)
	mode, _ := routeDecision["recommended_mode"].(string)
	if mode == "" || mode == "native_only" {
		return nil
	}
	limit := totalPages
	if maxPages > 0 && maxPages < limit {
		limit = maxPages
	}
	return sequentialProcessedPages(limit)
}

func limitPages(pages []int, maxPage int) []int {
	if len(pages) == 0 || maxPage <= 0 {
		return normalizeProcessedPages(pages)
	}
	out := make([]int, 0, len(pages))
	for _, page := range normalizeProcessedPages(pages) {
		if page <= maxPage {
			out = append(out, page)
		}
	}
	return out
}

func chunkItemsFromFullDoc(fullDoc map[string]any) []map[string]any {
	chW, ok := fullDoc["chunks"].(map[string]any)
	if !ok {
		return nil
	}
	return CoerceChunkItems(chW["items"])
}

func countNativeOnlyChunks(fullDoc map[string]any) int {
	return len(chunkItemsFromFullDoc(fullDoc))
}

func countNativeChunksPreservedWithVLM(items []map[string]any) int {
	return pdforchestrator.CountNativeChunksPreservedWithVLM(items)
}

func pageSetsEqual(a []int, b []int) bool {
	a = normalizeProcessedPages(a)
	b = normalizeProcessedPages(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func complementPages(totalPages int, appliedPages []int) []int {
	if totalPages <= 0 {
		return []int{}
	}
	applied := make(map[int]bool, len(appliedPages))
	for _, page := range normalizeProcessedPages(appliedPages) {
		applied[page] = true
	}
	out := make([]int, 0, totalPages-len(applied))
	for page := 1; page <= totalPages; page++ {
		if !applied[page] {
			out = append(out, page)
		}
	}
	return out
}

func subtractPages(basePages []int, removedPages []int) []int {
	base := normalizeProcessedPages(basePages)
	if len(base) == 0 {
		return []int{}
	}
	removed := pageMembershipSet(removedPages)
	out := make([]int, 0, len(base))
	for _, page := range base {
		if !removed[page] {
			out = append(out, page)
		}
	}
	return out
}

func pageMembershipSet(pages []int) map[int]bool {
	normalized := normalizeProcessedPages(pages)
	set := make(map[int]bool, len(normalized))
	for _, page := range normalized {
		set[page] = true
	}
	return set
}

func candidateReasonCodes(reasons []map[string]any) []string {
	if len(reasons) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		code, _ := reason["code"].(string)
		if code != "" {
			out = append(out, code)
		}
	}
	return out
}
