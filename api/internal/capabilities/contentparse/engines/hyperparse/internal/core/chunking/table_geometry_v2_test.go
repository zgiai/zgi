package chunking

import (
	"math"
	"testing"
)

func TestDeriveColumnAnchorsFromCellRows_MinMaxPerRow(t *testing.T) {
	// 模拟一行内大量微单元 minX 密排：旧逻辑会把整行宽度并成单一锚点。
	many := make([]cellRun, 0, 30)
	for i := 0; i < 30; i++ {
		x := 0.10 + float64(i)*0.02
		many = append(many, cellRun{points: []textPoint{{x: x, y: 0.5, bbox: &BBox{}}}})
	}
	row2 := make([]cellRun, 0, 30)
	for i := 0; i < 30; i++ {
		x := 0.10 + float64(i)*0.02
		row2 = append(row2, cellRun{points: []textPoint{{x: x, y: 0.48, bbox: &BBox{}}}})
	}
	rows := [][]cellRun{many, row2}
	anchors := deriveColumnAnchorsFromCellRows(rows)
	if len(anchors) < 2 {
		t.Fatalf("want >=2 anchors for two-column spread, got %v", anchors)
	}
	if math.Abs(anchors[len(anchors)-1]-anchors[0]) < 0.15 {
		t.Fatalf("anchors too close: %v", anchors)
	}
}
