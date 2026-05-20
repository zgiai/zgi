package chunking

import (
	"testing"
)

// Regression: when a multi-line parent is split into child segments, each child
// must receive an independent vertical slice of the parent bbox instead of
// inheriting the same parent anchor.

func TestSliceParentBBoxByLineRatio_EvenSplit(t *testing.T) {
	parent := &BBox{Left: 0.1, Top: 0.9, Right: 0.9, Bottom: 0.1}
	got0 := sliceParentBBoxByLineRatio(parent, 0, 1, 4)
	got3 := sliceParentBBoxByLineRatio(parent, 3, 1, 4)
	if got0 == nil || got3 == nil {
		t.Fatalf("expected non-nil slices, got %+v / %+v", got0, got3)
	}
	// Four equal slices over 0.8 height -> 0.2 each; top-origin with Top > Bottom.
	if got0.Top != 0.9 || got0.Bottom != 0.7 {
		t.Errorf("slice 0: Top=%v Bottom=%v want 0.9/0.7", got0.Top, got0.Bottom)
	}
	if got3.Top != 0.3 || got3.Bottom != 0.1 {
		t.Errorf("slice 3: Top=%v Bottom=%v want 0.3/0.1", got3.Top, got3.Bottom)
	}
	if got0.Left != 0.1 || got0.Right != 0.9 {
		t.Errorf("slice 0 horizontal drift: L=%v R=%v", got0.Left, got0.Right)
	}
}

func TestSliceParentBBoxByLineRatio_NilParent(t *testing.T) {
	if sliceParentBBoxByLineRatio(nil, 0, 1, 4) != nil {
		t.Fatalf("nil parent should return nil")
	}
}

func TestSliceParentBBoxByLineRatio_ZeroWidth(t *testing.T) {
	// Some adapters can emit zero-width boxes. Return nil so bad coordinates are
	// not inherited by every child line; downstream guards will downgrade them.
	parent := &BBox{Left: 1.0, Top: 0.6, Right: 1.0, Bottom: 0.5}
	if sliceParentBBoxByLineRatio(parent, 0, 1, 2) != nil {
		t.Fatalf("zero-width parent should return nil")
	}
}

func TestSliceParentBBoxByLineRatio_ZeroHeight(t *testing.T) {
	parent := &BBox{Left: 0.1, Top: 0.5, Right: 0.8, Bottom: 0.5}
	if sliceParentBBoxByLineRatio(parent, 0, 1, 2) != nil {
		t.Fatalf("zero-height parent should return nil")
	}
}

func TestSliceParentBBoxByLineRatio_WeightedBlock(t *testing.T) {
	// A four-line parent split into [one heading line, three body lines] should
	// use a 1:3 vertical ratio.
	parent := &BBox{Left: 0, Top: 1.0, Right: 1.0, Bottom: 0.0}
	heading := sliceParentBBoxByLineRatio(parent, 0, 1, 4)
	body := sliceParentBBoxByLineRatio(parent, 1, 3, 4)
	if heading.Bottom != 0.75 {
		t.Errorf("heading bottom=%v want 0.75", heading.Bottom)
	}
	if body.Top != 0.75 || body.Bottom != 0.0 {
		t.Errorf("body Top=%v Bottom=%v want 0.75/0.0", body.Top, body.Bottom)
	}
}

// End-to-end regression: sibling child lines produced by
// expandTextsForMultilineSegments must not share the same parent anchor box.
func TestExpandTextsForMultilineSegments_SubSegmentsGetDistinctBBoxes(t *testing.T) {
	parent := TextLike{
		Order:       0,
		SourceTrace: "page#1 obj#25",
		Text:        "CLINICAL DATA: R/O WART\nSPECIMEN: A.\nDIAGNOSIS:\nBIOPSY SHOWS\nFINDING A\nFINDING B",
		BBox:        &BBox{Left: 0.06, Top: 0.75, Right: 0.52, Bottom: 0.71},
	}
	out := expandTextsForMultilineSegments([]TextLike{parent})
	if len(out) < 3 {
		t.Fatalf("expected parent to split into >=3 blocks, got %d", len(out))
	}
	seen := map[string]int{}
	for i, tl := range out {
		if tl.BBox == nil {
			t.Fatalf("block %d (%q) has nil BBox, expected slice-of-parent fallback", i, tl.Text)
		}
		key := boxKey(tl.BBox)
		seen[key]++
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("bbox %s shared by %d blocks, expected each block to be unique", k, n)
		}
	}
}

func boxKey(b *BBox) string {
	return floatKey(b.Left) + "," + floatKey(b.Top) + "," + floatKey(b.Right) + "," + floatKey(b.Bottom)
}

func floatKey(v float64) string {
	// 精度 6，对齐 round() 调用。
	return formatF(v)
}

// 独立的数值格式化，避免依赖 fmt 引入测试噪声。
func formatF(v float64) string {
	// 简单 6 位小数字符串。
	const n = 1e6
	i := int64(v * n)
	return itoa(i)
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
