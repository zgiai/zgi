package layoutdoc

import (
	"math"
	"sort"
)

const (
	layoutGapMin             = 0.018
	wideElementMinWidth      = 0.64
	wideElementOverlapMin    = 2
	horizontalOverlapMin     = 0.10
	narrowOutlierMaxWidth    = 0.035
	narrowOutlierMaxHeight   = 0.055
	multiColumnOverlapMin    = 0.35
	topLeftSortYTolerance    = 0.012
	topLeftSortXTolerance    = 0.008
	crossLayoutWidthFraction = 0.72
)

type ReadingOrderProcessor struct{}

func NewReadingOrderProcessor() ReadingOrderProcessor {
	return ReadingOrderProcessor{}
}

func (ReadingOrderProcessor) Name() string {
	return "reading_order"
}

func (ReadingOrderProcessor) Process(doc *Document) (StageReport, error) {
	before := len(doc.Elements)
	doc.Elements = SortElementsByReadingOrder(doc.Elements)
	return StageReport{
		ID:     "reading_order",
		Status: "done",
		Count:  before,
		Detail: "xycut",
	}, nil
}

func sortXYCut(items []Element) []Element {
	if len(items) <= 1 {
		return append([]Element(nil), items...)
	}
	cross := identifyCrossLayoutElements(items)
	if len(cross) == len(items) {
		return sortTopLeft(items)
	}
	main := make([]Element, 0, len(items)-len(cross))
	crossSet := make(map[int]bool, len(cross))
	for _, item := range cross {
		crossSet[item.OriginalPos] = true
	}
	for _, item := range items {
		if !crossSet[item.OriginalPos] {
			main = append(main, item)
		}
	}
	sortedMain := recursiveSort(main)
	return mergeCrossLayoutElements(sortedMain, sortTopLeft(cross))
}

func recursiveSort(items []Element) []Element {
	if len(items) <= 2 {
		return sortTopLeft(items)
	}
	hCut, hGap := bestHorizontalCut(items)
	vCut, vGap := bestVerticalCut(items)
	hasH := hGap >= layoutGapMin
	hasV := vGap >= layoutGapMin
	if !hasH && !hasV {
		return sortTopLeft(items)
	}
	if hasV {
		left, right := splitVertical(items, vCut)
		if looksLikeMultiColumn(left, right) {
			out := recursiveSort(left)
			out = append(out, recursiveSort(right)...)
			return out
		}
	}
	if hasV && (!hasH || vGap > hGap) {
		left, right := splitVertical(items, vCut)
		if len(left) == 0 || len(right) == 0 {
			return sortTopLeft(items)
		}
		out := recursiveSort(left)
		out = append(out, recursiveSort(right)...)
		return out
	}
	above, below := splitHorizontal(items, hCut)
	if len(above) == 0 || len(below) == 0 {
		return sortTopLeft(items)
	}
	out := recursiveSort(above)
	out = append(out, recursiveSort(below)...)
	return out
}

func identifyCrossLayoutElements(items []Element) []Element {
	if len(items) < 3 {
		return nil
	}
	maxWidth := 0.0
	for _, item := range items {
		if item.BBox != nil {
			maxWidth = math.Max(maxWidth, item.BBox.Width())
		}
	}
	threshold := math.Max(wideElementMinWidth, maxWidth*crossLayoutWidthFraction)
	var out []Element
	for _, item := range items {
		if item.BBox == nil || item.BBox.Width() < threshold {
			continue
		}
		overlaps := 0
		for _, other := range items {
			if other.OriginalPos == item.OriginalPos || other.BBox == nil {
				continue
			}
			if horizontalOverlapRatio(*item.BBox, *other.BBox) >= horizontalOverlapMin {
				overlaps++
				if overlaps >= wideElementOverlapMin {
					out = append(out, item)
					break
				}
			}
		}
	}
	return out
}

func bestVerticalCut(items []Element) (float64, float64) {
	sorted := make([]Element, 0, len(items))
	for _, item := range items {
		if item.BBox == nil {
			continue
		}
		if item.BBox.Width() <= narrowOutlierMaxWidth && item.BBox.Height() <= narrowOutlierMaxHeight {
			continue
		}
		sorted = append(sorted, item)
	}
	if len(sorted) < 2 {
		sorted = append([]Element(nil), items...)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].BBox.Left != sorted[j].BBox.Left {
			return sorted[i].BBox.Left < sorted[j].BBox.Left
		}
		return sorted[i].BBox.Right < sorted[j].BBox.Right
	})
	largestGap := 0.0
	cut := 0.0
	prevRight := sorted[0].BBox.Right
	for i := 1; i < len(sorted); i++ {
		left := sorted[i].BBox.Left
		if left > prevRight {
			gap := left - prevRight
			if gap > largestGap {
				largestGap = gap
				cut = (left + prevRight) / 2
			}
		}
		prevRight = math.Max(prevRight, sorted[i].BBox.Right)
	}
	return cut, largestGap
}

func bestHorizontalCut(items []Element) (float64, float64) {
	sorted := append([]Element(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].BBox.Top != sorted[j].BBox.Top {
			return sorted[i].BBox.Top < sorted[j].BBox.Top
		}
		return sorted[i].BBox.Bottom < sorted[j].BBox.Bottom
	})
	largestGap := 0.0
	cut := 0.0
	prevBottom := sorted[0].BBox.Bottom
	for i := 1; i < len(sorted); i++ {
		top := sorted[i].BBox.Top
		if top > prevBottom {
			gap := top - prevBottom
			if gap > largestGap {
				largestGap = gap
				cut = (top + prevBottom) / 2
			}
		}
		prevBottom = math.Max(prevBottom, sorted[i].BBox.Bottom)
	}
	return cut, largestGap
}

func splitVertical(items []Element, cut float64) ([]Element, []Element) {
	var left, right []Element
	for _, item := range items {
		if item.BBox.CenterX() < cut {
			left = append(left, item)
		} else {
			right = append(right, item)
		}
	}
	return left, right
}

func splitHorizontal(items []Element, cut float64) ([]Element, []Element) {
	var above, below []Element
	for _, item := range items {
		if item.BBox.CenterY() < cut {
			above = append(above, item)
		} else {
			below = append(below, item)
		}
	}
	return above, below
}

func looksLikeMultiColumn(left, right []Element) bool {
	if len(left) < 2 || len(right) < 2 {
		return false
	}
	leftTop, leftBottom := verticalSpan(left)
	rightTop, rightBottom := verticalSpan(right)
	overlap := math.Min(leftBottom, rightBottom) - math.Max(leftTop, rightTop)
	if overlap <= 0 {
		return false
	}
	shorter := math.Min(leftBottom-leftTop, rightBottom-rightTop)
	return shorter > 0 && overlap/shorter >= multiColumnOverlapMin
}

func verticalSpan(items []Element) (float64, float64) {
	if len(items) == 0 || items[0].BBox == nil {
		return 0, 0
	}
	top := items[0].BBox.Top
	bottom := items[0].BBox.Bottom
	for _, item := range items[1:] {
		if item.BBox == nil {
			continue
		}
		top = math.Min(top, item.BBox.Top)
		bottom = math.Max(bottom, item.BBox.Bottom)
	}
	return top, bottom
}

func mergeCrossLayoutElements(main, cross []Element) []Element {
	if len(cross) == 0 {
		return main
	}
	if len(main) == 0 {
		return cross
	}
	out := make([]Element, 0, len(main)+len(cross))
	i, j := 0, 0
	for i < len(main) || j < len(cross) {
		if j >= len(cross) {
			out = append(out, main[i])
			i++
			continue
		}
		if i >= len(main) {
			out = append(out, cross[j])
			j++
			continue
		}
		if cross[j].BBox.Top <= main[i].BBox.Top {
			out = append(out, cross[j])
			j++
		} else {
			out = append(out, main[i])
			i++
		}
	}
	return out
}

func sortTopLeft(items []Element) []Element {
	out := append([]Element(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		if math.Abs(out[i].BBox.Top-out[j].BBox.Top) > topLeftSortYTolerance {
			return out[i].BBox.Top < out[j].BBox.Top
		}
		if math.Abs(out[i].BBox.Left-out[j].BBox.Left) > topLeftSortXTolerance {
			return out[i].BBox.Left < out[j].BBox.Left
		}
		return out[i].OriginalPos < out[j].OriginalPos
	})
	return out
}

func horizontalOverlapRatio(a, b BBox) float64 {
	left := math.Max(a.Left, b.Left)
	right := math.Min(a.Right, b.Right)
	overlap := math.Max(0, right-left)
	if overlap <= 0 {
		return 0
	}
	smaller := math.Min(a.Width(), b.Width())
	if smaller <= 0 {
		return 0
	}
	return overlap / smaller
}
