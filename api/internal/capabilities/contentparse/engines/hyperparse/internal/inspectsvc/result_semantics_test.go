package inspectsvc

import "testing"

func TestBuildInspectSemanticsNativeOnly(t *testing.T) {
	scope, coverage, merge := buildInspectSemantics(3, nil, nil, 7, 0, false, nil)
	if scope != "full" {
		t.Fatalf("scope=%q", scope)
	}
	if complete, _ := coverage["complete"].(bool); !complete {
		t.Fatalf("coverage.complete=%v, want true", coverage["complete"])
	}
	if got, _ := merge["strategy"].(string); got != "native_only" {
		t.Fatalf("merge.strategy=%q", got)
	}
	if applied, _ := merge["applied"].(bool); applied {
		t.Fatalf("merge.applied=%v, want false", merge["applied"])
	}
	if got := len(merge["native_pages_kept"].([]int)); got != 3 {
		t.Fatalf("native_pages_kept len=%d", got)
	}
}

func TestBuildInspectSemanticsPreviewPartial(t *testing.T) {
	scope, coverage, merge := buildInspectSemantics(5, []int{1, 2, 4}, []int{1, 2}, 2, 4, true, []int{1, 2})
	if scope != "preview_partial" {
		t.Fatalf("scope=%q", scope)
	}
	if complete, _ := coverage["complete"].(bool); complete {
		t.Fatalf("coverage.complete=%v, want false", coverage["complete"])
	}
	if got, _ := merge["strategy"].(string); got != "preview_partial_vlm" {
		t.Fatalf("merge.strategy=%q", got)
	}
}

func TestBuildInspectSemanticsForcePreviewPartial(t *testing.T) {
	scope, coverage, merge := buildInspectSemantics(2, []int{1, 2}, []int{1, 2}, 1, 2, true, []int{1, 2})
	if scope != "preview_partial" {
		t.Fatalf("scope=%q", scope)
	}
	if complete, _ := coverage["complete"].(bool); !complete {
		t.Fatalf("coverage.complete=%v, want true", coverage["complete"])
	}
	if got, _ := merge["strategy"].(string); got != "preview_partial_vlm" {
		t.Fatalf("merge.strategy=%q", got)
	}
}

func TestBuildInspectSemanticsFullVLMMerge(t *testing.T) {
	scope, coverage, merge := buildInspectSemantics(2, []int{1, 2}, []int{1, 2}, 1, 3, false, []int{1, 2})
	if scope != "full" {
		t.Fatalf("scope=%q", scope)
	}
	if complete, _ := coverage["complete"].(bool); !complete {
		t.Fatalf("coverage.complete=%v, want true", coverage["complete"])
	}
	if got, _ := merge["strategy"].(string); got != "full_vlm_merge" {
		t.Fatalf("merge.strategy=%q", got)
	}
	if applied, _ := merge["applied"].(bool); !applied {
		t.Fatalf("merge.applied=%v, want true", merge["applied"])
	}
}

func TestBuildInspectSemanticsFullResultWithPageSubset(t *testing.T) {
	scope, coverage, merge := buildInspectSemantics(5, []int{2, 4}, []int{2, 4}, 3, 6, false, []int{2, 4})
	if scope != "full" {
		t.Fatalf("scope=%q", scope)
	}
	if complete, _ := coverage["complete"].(bool); !complete {
		t.Fatalf("coverage.complete=%v, want true", coverage["complete"])
	}
	if got, _ := merge["strategy"].(string); got != "full_vlm_merge" {
		t.Fatalf("merge.strategy=%q", got)
	}
	if got := coverage["planned"].([]int); len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Fatalf("coverage.planned=%v", got)
	}
}

func TestBuildRouteDebug(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"route_decision": map[string]any{
				"recommended_mode": "vlm_candidate",
			},
			"page_route_candidates": []map[string]any{
				{
					"page_index":       2,
					"recommended_mode": "vlm_candidate",
					"score":            0.81,
					"reasons": []map[string]any{
						{"code": "business_form_like", "score": 0.81},
					},
					"native_signals": map[string]any{
						"kv_line_count": 4,
					},
				},
				{
					"page_index":       3,
					"recommended_mode": "native_only",
				},
			},
		},
	}

	debug := buildRouteDebug(fullDoc, []int{2, 4}, []int{2}, []int{2})
	if got := debug["candidate_pages"].([]int); len(got) != 1 || got[0] != 2 {
		t.Fatalf("candidate_pages=%v", got)
	}
	if got := debug["pending_candidate_pages"].([]int); len(got) != 1 || got[0] != 4 {
		t.Fatalf("pending_candidate_pages=%v", got)
	}
	candidates := debug["candidate_details"].([]map[string]any)
	if len(candidates) != 1 {
		t.Fatalf("candidate_details=%v", candidates)
	}
	if got, _ := candidates[0]["processed_for_vlm"].(bool); !got {
		t.Fatalf("processed_for_vlm=%v", candidates[0]["processed_for_vlm"])
	}
	if got, _ := candidates[0]["applied_vlm"].(bool); !got {
		t.Fatalf("applied_vlm=%v", candidates[0]["applied_vlm"])
	}
	reasonCodes := candidates[0]["reason_codes"].([]string)
	if len(reasonCodes) != 1 || reasonCodes[0] != "business_form_like" {
		t.Fatalf("reason_codes=%v", reasonCodes)
	}
}
