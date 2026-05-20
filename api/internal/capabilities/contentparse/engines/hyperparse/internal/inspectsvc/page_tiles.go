package inspectsvc

import "math"

type normalizedPageRect struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

type normalizedCropRect = normalizedPageRect

type pageTileRef struct {
	PageIndex int
	TileIndex int
	Crop      normalizedPageRect
	Coverage  normalizedPageRect
}

func (r normalizedPageRect) valid() bool {
	return r.Left >= 0 && r.Top >= 0 && r.Right <= 1 && r.Bottom <= 1 && r.Right > r.Left && r.Bottom > r.Top
}

func (r normalizedPageRect) width() float64 {
	return r.Right - r.Left
}

func (r normalizedPageRect) height() float64 {
	return r.Bottom - r.Top
}

func (r normalizedPageRect) bboxMap() map[string]any {
	return map[string]any{
		"left":   r.Left,
		"top":    r.Top,
		"right":  r.Right,
		"bottom": r.Bottom,
	}
}

func (r normalizedPageRect) overlaps(left, top, right, bottom float64) bool {
	return left < r.Right && right > r.Left && top < r.Bottom && bottom > r.Top
}

func (r normalizedPageRect) overlapSize(left, top, right, bottom float64) (width, height float64) {
	width = math.Min(r.Right, right) - math.Max(r.Left, left)
	height = math.Min(r.Bottom, bottom) - math.Max(r.Top, top)
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return width, height
}

func (r normalizedPageRect) mapNormalizedBox(left, top, right, bottom float64) (normalizedPageRect, bool) {
	if !r.valid() || right <= left || bottom <= top {
		return normalizedPageRect{}, false
	}
	if left < 0 || top < 0 || right > 1 || bottom > 1 {
		return normalizedPageRect{}, false
	}
	mapped := normalizedPageRect{
		Left:   r.Left + left*r.width(),
		Top:    r.Top + top*r.height(),
		Right:  r.Left + right*r.width(),
		Bottom: r.Top + bottom*r.height(),
	}
	if !mapped.valid() {
		return normalizedPageRect{}, false
	}
	return mapped, true
}

func (r normalizedPageRect) payloadMap() map[string]any {
	return r.bboxMap()
}

func (t pageTileRef) payloadMap() map[string]any {
	return map[string]any{
		"page_index": t.PageIndex,
		"tile_index": t.TileIndex,
		"crop":       t.Crop.payloadMap(),
		"coverage":   t.Coverage.payloadMap(),
	}
}

func SplitPageIntoHorizontalTiles(pageIndex, count int, overlap float64) []pageTileRef {
	return SplitPageIntoGridTiles(pageIndex, count, 1, overlap, 0)
}

func SplitPageIntoGridTiles(pageIndex, cols, rows int, overlapX, overlapY float64) []pageTileRef {
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}
	overlapX = clampNormalizedOverlap(overlapX)
	overlapY = clampNormalizedOverlap(overlapY)

	coverageW := 1 / float64(cols)
	coverageH := 1 / float64(rows)
	out := make([]pageTileRef, 0, cols*rows)
	tileIndex := 1
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			coverage := normalizedPageRect{
				Left:   float64(col) * coverageW,
				Top:    float64(row) * coverageH,
				Right:  float64(col+1) * coverageW,
				Bottom: float64(row+1) * coverageH,
			}
			crop := normalizedPageRect{
				Left:   math.Max(0, coverage.Left-overlapX),
				Top:    math.Max(0, coverage.Top-overlapY),
				Right:  math.Min(1, coverage.Right+overlapX),
				Bottom: math.Min(1, coverage.Bottom+overlapY),
			}
			out = append(out, pageTileRef{
				PageIndex: pageIndex,
				TileIndex: tileIndex,
				Crop:      crop,
				Coverage:  coverage,
			})
			tileIndex++
		}
	}
	return out
}

func RemapChunkBBoxFromTile(chunk map[string]any, tile pageTileRef) bool {
	left, top, right, bottom, ok := chunkBBox(chunk)
	if !ok {
		return false
	}
	mapped, ok := tile.Crop.mapNormalizedBox(left, top, right, bottom)
	if !ok {
		return false
	}
	chunk["bbox"] = mapped.bboxMap()
	attachChunkTileRef(chunk, tile)
	return true
}

func attachChunkTileRef(chunk map[string]any, tile pageTileRef) {
	payload, _ := chunk["payload"].(map[string]any)
	if payload == nil {
		payload = map[string]any{}
		chunk["payload"] = payload
	}
	payload["tile_ref"] = tile.payloadMap()
}

func clampNormalizedOverlap(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 0.49 {
		return 0.49
	}
	return v
}
