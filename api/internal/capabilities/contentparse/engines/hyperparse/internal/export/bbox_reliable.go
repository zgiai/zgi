package export

import "fmt"

// Box is a small internal value type used by dpt_export.
type Box struct {
	Left, Top, Right, Bottom float64
}

// IsBBoxReliableGeom checks geometric validity without considering shared-box reuse.
// Criteria: non-zero dimensions, reasonable area, and 1% boundary tolerance.
func IsBBoxReliableGeom(b Box) bool {
	if b.Right <= b.Left || b.Bottom <= b.Top {
		return false
	}
	area := (b.Right - b.Left) * (b.Bottom - b.Top)
	if area < 0.0001 {
		return false
	}
	if area > 0.72 {
		return false
	}
	if b.Left < -0.01 || b.Right > 1.01 {
		return false
	}
	if b.Top < -0.01 || b.Bottom > 1.01 {
		return false
	}
	return true
}

// bboxKey discretizes a normalized bbox for shared-box counting.
func bboxKey(b Box) string {
	return fmt.Sprintf("%.4f,%.4f,%.4f,%.4f", b.Left, b.Top, b.Right, b.Bottom)
}

// bboxFromCanonicalMap reads a Box from a canonical grounding map.
func bboxFromCanonicalMap(m map[string]any) (Box, bool) {
	if m == nil {
		return Box{}, false
	}
	b := Box{
		Left:   toF64(m["left"]),
		Top:    toF64(m["top"]),
		Right:  toF64(m["right"]),
		Bottom: toF64(m["bottom"]),
	}
	if b.Left == 0 && b.Top == 0 && b.Right == 0 && b.Bottom == 0 {
		return Box{}, false
	}
	return b, true
}

// BBoxReliabilityReport summarizes bbox reliability during DPT export.
type BBoxReliabilityReport struct {
	TotalChunks      int            `json:"total_chunks"`
	WithBox          int            `json:"with_box"`
	GeomReliable     int            `json:"geom_reliable"`
	SharedBoxOver3   int            `json:"shared_box_over_3"` // number of boxes shared by more than three chunks
	UniqueBoxes      int            `json:"unique_boxes"`
	ReliableChunkIDs []string       `json:"-"`
	Downgraded       []string       `json:"downgraded"` // chunk IDs marked unreliable
	SharedBoxTop     map[string]int `json:"shared_box_top"`
}

// ClassifyBBoxReliability returns a reliability report plus an unreliable chunk-id set.
func ClassifyBBoxReliability(boxesByChunk map[string]map[string]any) (BBoxReliabilityReport, map[string]bool) {
	report := BBoxReliabilityReport{SharedBoxTop: map[string]int{}}
	unreliable := map[string]bool{}

	// Step 1: count how often each bbox appears.
	counts := map[string]int{}
	boxByChunk := map[string]Box{}
	for id, m := range boxesByChunk {
		report.TotalChunks++
		b, ok := bboxFromCanonicalMap(m)
		if !ok {
			continue
		}
		report.WithBox++
		key := bboxKey(b)
		counts[key]++
		boxByChunk[id] = b
	}
	report.UniqueBoxes = len(counts)

	// Step 2: evaluate per chunk.
	for id, b := range boxByChunk {
		reliable := true
		if !IsBBoxReliableGeom(b) {
			reliable = false
		}
		key := bboxKey(b)
		if counts[key] > 3 {
			reliable = false
			// Record shared bboxes for reporting, capped to the first 10.
			if _, seen := report.SharedBoxTop[key]; !seen && len(report.SharedBoxTop) < 10 {
				report.SharedBoxTop[key] = counts[key]
			}
		}
		if reliable {
			report.GeomReliable++
			report.ReliableChunkIDs = append(report.ReliableChunkIDs, id)
		} else {
			unreliable[id] = true
			report.Downgraded = append(report.Downgraded, id)
		}
	}

	// Step 3: SharedBoxOver3 counts bboxes shared by more than three chunks.
	for _, c := range counts {
		if c > 3 {
			report.SharedBoxOver3++
		}
	}
	return report, unreliable
}
