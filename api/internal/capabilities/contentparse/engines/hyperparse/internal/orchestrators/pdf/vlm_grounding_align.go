package pdf

import (
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
)

var vlmTextGroundingTypes = map[string]bool{
	"paragraph":  true,
	"heading":    true,
	"kv":         true,
	"list_item":  true,
	"formula":    true,
	"annotation": true,
	"other":      true,
}

type vlmGroundingCandidate struct {
	ChunkID    string
	PageIndex  int
	Order      int
	Type       string
	Text       string
	BBoxRaw    map[string]any
	BBoxUIRect vlmBBoxRect
}

type vlmGroundingIndex struct {
	TextByPage  map[int][]vlmGroundingCandidate
	TableByPage map[int][]vlmGroundingCandidate
	ImageByPage map[int][]vlmGroundingCandidate
}

type vlmBBoxRect struct {
	Left   float64
	Right  float64
	Top    float64
	Bottom float64
}

func alignVLMChunkGrounding(native []map[string]any, vlm []map[string]any) []map[string]any {
	if len(vlm) == 0 {
		return nil
	}
	index := buildVLMGroundingIndex(native)
	out := make([]map[string]any, 0, len(vlm))
	for _, raw := range vlm {
		chunk := cloneChunkMap(raw)
		annotateInitialVLMBBoxState(chunk)
		page := chunkPageIndex(chunk)
		typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(chunk["type"])))
		switch {
		case typ == "table":
			if cand, ok := bestVLMGroundingCandidate(chunk, index.TableByPage[page], true); ok {
				applyAlignedVLMGrounding(chunk, cand, vlmGroundingSource(cand, "native_table_align"))
			}
		case typ == "image":
			if cand, ok := bestVLMImageGroundingCandidate(chunk, index.ImageByPage[page]); ok {
				applyAlignedVLMGrounding(chunk, cand, "native_image_align")
			}
		case vlmTextGroundingTypes[typ]:
			if cand, ok := bestVLMGroundingCandidate(chunk, index.TextByPage[page], false); ok {
				applyAlignedVLMGrounding(chunk, cand, vlmGroundingSource(cand, "native_text_align"))
			}
		}
		out = append(out, chunk)
	}
	return out
}

func buildVLMGroundingIndex(native []map[string]any) vlmGroundingIndex {
	idx := vlmGroundingIndex{
		TextByPage:  map[int][]vlmGroundingCandidate{},
		TableByPage: map[int][]vlmGroundingCandidate{},
		ImageByPage: map[int][]vlmGroundingCandidate{},
	}
	for _, chunk := range native {
		page := chunkPageIndex(chunk)
		if page < 1 {
			continue
		}
		bb, ok := chunk["bbox"].(map[string]any)
		if !ok || len(bb) == 0 {
			continue
		}
		rect, ok := canonicalVLMBBoxRect(bb)
		if !ok {
			continue
		}
		cand := vlmGroundingCandidate{
			ChunkID:    strings.TrimSpace(fmt.Sprint(chunk["chunk_id"])),
			PageIndex:  page,
			Order:      intFromAny(chunk["order"]),
			Type:       strings.ToLower(strings.TrimSpace(fmt.Sprint(chunk["type"]))),
			Text:       comparableChunkText(chunk),
			BBoxRaw:    cloneBBoxMap(bb),
			BBoxUIRect: rect,
		}
		switch cand.Type {
		case "table":
			idx.TableByPage[page] = append(idx.TableByPage[page], cand)
		case "image", "stamp":
			idx.ImageByPage[page] = append(idx.ImageByPage[page], cand)
		case "bbox_anchor":
			idx.TextByPage[page] = append(idx.TextByPage[page], cand)
			idx.TableByPage[page] = append(idx.TableByPage[page], cand)
		default:
			if cand.Text == "" {
				continue
			}
			idx.TextByPage[page] = append(idx.TextByPage[page], cand)
		}
	}
	for page := range idx.TextByPage {
		sort.SliceStable(idx.TextByPage[page], func(i, j int) bool {
			return idx.TextByPage[page][i].Order < idx.TextByPage[page][j].Order
		})
	}
	for page := range idx.TableByPage {
		sort.SliceStable(idx.TableByPage[page], func(i, j int) bool {
			return idx.TableByPage[page][i].Order < idx.TableByPage[page][j].Order
		})
	}
	for page := range idx.ImageByPage {
		sort.SliceStable(idx.ImageByPage[page], func(i, j int) bool {
			return idx.ImageByPage[page][i].Order < idx.ImageByPage[page][j].Order
		})
	}
	return idx
}

func annotateInitialVLMBBoxState(chunk map[string]any) {
	if _, exists := chunk["bbox_precise"]; !exists {
		chunk["bbox_precise"] = false
	}
	if bb, ok := chunk["bbox"].(map[string]any); ok && len(bb) > 0 {
		chunk["bbox_source"] = "vlm_bbox_raw"
		if _, exists := chunk["bbox_confidence"]; !exists {
			chunk["bbox_confidence"] = 0.58
		}
		payload := ensureChunkPayload(chunk)
		if _, exists := payload["vlm_raw_bbox"]; !exists {
			payload["vlm_raw_bbox"] = cloneBBoxMap(bb)
		}
	}
}

func bestVLMGroundingCandidate(chunk map[string]any, candidates []vlmGroundingCandidate, tableOnly bool) (vlmGroundingCandidate, bool) {
	if len(candidates) == 0 {
		return vlmGroundingCandidate{}, false
	}
	chunkText := comparableChunkText(chunk)
	if cand, ok := bestVLMUnionGroundingCandidate(chunkText, candidates, tableOnly); ok {
		return cand, true
	}
	if tableOnly && len(candidates) == 1 {
		return candidates[0], true
	}
	rawRect, hasRawRect := canonicalBBoxFromChunk(chunk)
	bestScore := 0.0
	bestIdx := -1
	chunkType := strings.ToLower(strings.TrimSpace(fmt.Sprint(chunk["type"])))
	for i, cand := range candidates {
		score := comparableTextSimilarity(chunkText, cand.Text)
		if cand.Type == chunkType {
			score += 0.08
		} else if tableOnly && cand.Type == "table" {
			score += 0.04
		} else if !tableOnly && vlmTextGroundingTypes[cand.Type] {
			score += 0.03
		}
		if hasRawRect {
			score += 0.15 * bboxIoU(rawRect, cand.BBoxUIRect)
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	threshold := 0.74
	if tableOnly {
		threshold = 0.52
	}
	if bestIdx < 0 || bestScore < threshold {
		return vlmGroundingCandidate{}, false
	}
	return candidates[bestIdx], true
}

func bestVLMUnionGroundingCandidate(chunkText string, candidates []vlmGroundingCandidate, tableOnly bool) (vlmGroundingCandidate, bool) {
	if chunkText == "" || len(candidates) < 2 {
		return vlmGroundingCandidate{}, false
	}
	type match struct {
		cand vlmGroundingCandidate
		key  string
	}
	matches := make([]match, 0, len(candidates))
	seen := map[string]bool{}
	coveredRunes := 0
	for _, cand := range candidates {
		if cand.Text == "" || len(cand.BBoxRaw) == 0 {
			continue
		}
		if tableOnly && cand.Type != "table" && cand.Type != "bbox_anchor" {
			continue
		}
		if !tableOnly && cand.Type == "table" {
			continue
		}
		shorter := cand.Text
		longer := chunkText
		if utf8Len(longer) < utf8Len(shorter) {
			shorter, longer = longer, shorter
		}
		if utf8Len(shorter) < 4 || !strings.Contains(longer, shorter) {
			continue
		}
		key := fmt.Sprintf("%d:%s", cand.Order, cand.Text)
		if seen[key] {
			continue
		}
		seen[key] = true
		matches = append(matches, match{cand: cand, key: key})
		coveredRunes += utf8Len(shorter)
	}
	if len(matches) < 2 {
		return vlmGroundingCandidate{}, false
	}
	coverage := float64(coveredRunes) / float64(maxInt(utf8Len(chunkText), 1))
	minCoverage := 0.22
	if tableOnly {
		minCoverage = 0.12
	}
	if coverage < minCoverage && len(matches) < 4 {
		return vlmGroundingCandidate{}, false
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].cand.Order < matches[j].cand.Order
	})
	unionRaw := cloneBBoxMap(matches[0].cand.BBoxRaw)
	unionRect := matches[0].cand.BBoxUIRect
	ids := make([]string, 0, len(matches))
	for i, m := range matches {
		ids = append(ids, m.cand.ChunkID)
		if i == 0 {
			continue
		}
		unionRaw = unionBBoxMaps(unionRaw, m.cand.BBoxRaw)
		unionRect = unionVLMRects(unionRect, m.cand.BBoxUIRect)
	}
	typ := "bbox_anchor_union"
	if tableOnly {
		typ = "table_anchor_union"
	}
	return vlmGroundingCandidate{
		ChunkID:    strings.Join(ids, ","),
		PageIndex:  matches[0].cand.PageIndex,
		Order:      matches[0].cand.Order,
		Type:       typ,
		Text:       chunkText,
		BBoxRaw:    unionRaw,
		BBoxUIRect: unionRect,
	}, true
}

func bestVLMImageGroundingCandidate(chunk map[string]any, candidates []vlmGroundingCandidate) (vlmGroundingCandidate, bool) {
	if len(candidates) == 0 {
		return vlmGroundingCandidate{}, false
	}
	rawRect, hasRawRect := canonicalBBoxFromChunk(chunk)
	if len(candidates) == 1 && !hasRawRect {
		return candidates[0], true
	}
	bestScore := 0.0
	bestIdx := -1
	for i, cand := range candidates {
		score := 0.0
		if hasRawRect {
			score = bboxIoU(rawRect, cand.BBoxUIRect)
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 || bestScore < 0.32 {
		return vlmGroundingCandidate{}, false
	}
	return candidates[bestIdx], true
}

func applyAlignedVLMGrounding(chunk map[string]any, cand vlmGroundingCandidate, source string) {
	if chunk == nil || len(cand.BBoxRaw) == 0 {
		return
	}
	payload := ensureChunkPayload(chunk)
	if bb, ok := chunk["bbox"].(map[string]any); ok && len(bb) > 0 {
		if _, exists := payload["vlm_raw_bbox"]; !exists {
			payload["vlm_raw_bbox"] = cloneBBoxMap(bb)
		}
	}
	payload["bbox_aligned_from_chunk_id"] = cand.ChunkID
	payload["bbox_aligned_from_type"] = cand.Type
	payload["bbox_source"] = source
	payload["bbox_precise"] = true
	payload["bbox_confidence"] = 0.98
	chunk["bbox"] = cloneBBoxMap(cand.BBoxRaw)
	chunk["bbox_source"] = source
	chunk["bbox_precise"] = true
	chunk["bbox_confidence"] = 0.98
}

func vlmGroundingSource(cand vlmGroundingCandidate, fallback string) string {
	switch cand.Type {
	case "bbox_anchor":
		return "ocr_bbox_anchor"
	case "bbox_anchor_union":
		return "ocr_bbox_anchor_union"
	case "table_anchor_union":
		return "ocr_table_bbox_anchor_union"
	default:
		return fallback
	}
}

func comparableChunkText(chunk map[string]any) string {
	if chunk == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(chunk["text"]))
	if text == "" {
		return ""
	}
	text = html.UnescapeString(text)
	text = comparableHTMLTagRE.ReplaceAllString(text, " ")
	return normalizeComparableText(text)
}

func comparableTextSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	contain := containmentSimilarity(a, b)
	dice := runeBigramDiceCoefficient(a, b)
	if contain > dice {
		return contain
	}
	return dice
}

func containmentSimilarity(a, b string) float64 {
	longer := a
	shorter := b
	if utf8Len(shorter) > utf8Len(longer) {
		longer, shorter = shorter, longer
	}
	if shorter == "" || !strings.Contains(longer, shorter) {
		return 0
	}
	return float64(utf8Len(shorter)) / float64(utf8Len(longer))
}

func runeBigramDiceCoefficient(a, b string) float64 {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	if len(ar) == 1 || len(br) == 1 {
		if a == b {
			return 1
		}
		return 0
	}
	ab := runeBigrams(ar)
	bb := runeBigrams(br)
	if len(ab) == 0 || len(bb) == 0 {
		return 0
	}
	used := make([]bool, len(bb))
	matches := 0
	for _, ga := range ab {
		for j, gb := range bb {
			if used[j] || ga != gb {
				continue
			}
			used[j] = true
			matches++
			break
		}
	}
	return (2 * float64(matches)) / float64(len(ab)+len(bb))
}

func runeBigrams(runes []rune) []string {
	if len(runes) < 2 {
		return nil
	}
	out := make([]string, 0, len(runes)-1)
	for i := 0; i+1 < len(runes); i++ {
		out = append(out, string([]rune{runes[i], runes[i+1]}))
	}
	return out
}

func utf8Len(s string) int {
	return len([]rune(s))
}

func canonicalBBoxFromChunk(chunk map[string]any) (vlmBBoxRect, bool) {
	bb, ok := chunk["bbox"].(map[string]any)
	if !ok || len(bb) == 0 {
		return vlmBBoxRect{}, false
	}
	return canonicalVLMBBoxRect(bb)
}

func canonicalVLMBBoxRect(bb map[string]any) (vlmBBoxRect, bool) {
	l := numFromMap(bb, "left")
	r := numFromMap(bb, "right")
	t := numFromMap(bb, "top")
	b := numFromMap(bb, "bottom")
	if l == 0 && r == 0 && t == 0 && b == 0 {
		return vlmBBoxRect{}, false
	}
	if r < l {
		l, r = r, l
	}
	top := t
	bottom := b
	if t > b {
		top = 1 - t
		bottom = 1 - b
	}
	if bottom < top {
		top, bottom = bottom, top
	}
	return vlmBBoxRect{
		Left:   clampRectCoord(l),
		Right:  clampRectCoord(r),
		Top:    clampRectCoord(top),
		Bottom: clampRectCoord(bottom),
	}, true
}

func bboxIoU(a, b vlmBBoxRect) float64 {
	interLeft := math.Max(a.Left, b.Left)
	interRight := math.Min(a.Right, b.Right)
	interTop := math.Max(a.Top, b.Top)
	interBottom := math.Min(a.Bottom, b.Bottom)
	if interRight <= interLeft || interBottom <= interTop {
		return 0
	}
	interArea := (interRight - interLeft) * (interBottom - interTop)
	aArea := math.Max(0, a.Right-a.Left) * math.Max(0, a.Bottom-a.Top)
	bArea := math.Max(0, b.Right-b.Left) * math.Max(0, b.Bottom-b.Top)
	unionArea := aArea + bArea - interArea
	if unionArea <= 0 {
		return 0
	}
	return interArea / unionArea
}

func unionVLMRects(a, b vlmBBoxRect) vlmBBoxRect {
	return vlmBBoxRect{
		Left:   math.Min(a.Left, b.Left),
		Right:  math.Max(a.Right, b.Right),
		Top:    math.Min(a.Top, b.Top),
		Bottom: math.Max(a.Bottom, b.Bottom),
	}
}

func unionBBoxMaps(a, b map[string]any) map[string]any {
	ar, aok := canonicalVLMBBoxRect(a)
	br, bok := canonicalVLMBBoxRect(b)
	if !aok && !bok {
		return nil
	}
	if !aok {
		return cloneBBoxMap(b)
	}
	if !bok {
		return cloneBBoxMap(a)
	}
	u := unionVLMRects(ar, br)
	return map[string]any{
		"left":   u.Left,
		"right":  u.Right,
		"top":    u.Top,
		"bottom": u.Bottom,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampRectCoord(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func cloneBBoxMap(bb map[string]any) map[string]any {
	if len(bb) == 0 {
		return nil
	}
	out := make(map[string]any, len(bb))
	for k, v := range bb {
		out[k] = v
	}
	return out
}

func ensureChunkPayload(chunk map[string]any) map[string]any {
	if chunk == nil {
		return nil
	}
	if payload, ok := chunk["payload"].(map[string]any); ok && payload != nil {
		return payload
	}
	payload := map[string]any{}
	chunk["payload"] = payload
	return payload
}
