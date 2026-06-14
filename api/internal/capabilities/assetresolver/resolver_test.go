package assetresolver

import (
	"strings"
	"testing"
)

func TestResolverPrefersSelectedFile(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "first.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "chosen.pdf", "extension": "pdf", "selected": true},
			},
		},
		Selectors: []Selector{{Type: "file"}},
	})

	assertResolvedAssetIDs(t, result, "file-2")
	if got := result.Resolutions[0].Assets[0].Metadata["selected"]; got != true {
		t.Fatalf("selected metadata = %#v, want true", got)
	}
}

func TestResolverResolvesVisibleOrdinalOneBased(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"visible_files": []interface{}{
				map[string]interface{}{"file_id": "file-1", "name": "one.txt"},
				map[string]interface{}{"file_id": "file-2", "name": "two.txt"},
				map[string]interface{}{"file_id": "file-3", "name": "three.txt"},
				map[string]interface{}{"file_id": "file-4", "name": "four.txt"},
				map[string]interface{}{"file_id": "file-5", "name": "five.txt"},
			},
		},
		Selectors: []Selector{{Type: "file", Selector: "visible_files[4]"}},
	})

	assertResolvedAssetIDs(t, result, "file-4")
	if got := result.Resolutions[0].Assets[0].Metadata["visible_ordinal"]; got != 4 {
		t.Fatalf("visible_ordinal = %#v, want 4", got)
	}
}

func TestResolverResolvesChineseVisibleOrdinalSelector(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"visible_files": []interface{}{
				map[string]interface{}{"file_id": "file-1", "name": "one.txt"},
				map[string]interface{}{"file_id": "file-2", "name": "two.txt"},
				map[string]interface{}{"file_id": "file-3", "name": "three.txt"},
				map[string]interface{}{"file_id": "file-4", "name": "four.txt"},
				map[string]interface{}{"file_id": "file-5", "name": "five.txt"},
			},
		},
		Selectors: []Selector{{Type: "file", Selector: "\u7b2c\u56db\u4e2a\u6587\u4ef6"}},
	})

	assertResolvedAssetIDs(t, result, "file-4")
	if got := result.Resolutions[0].Assets[0].Metadata["visible_ordinal"]; got != 4 {
		t.Fatalf("visible_ordinal = %#v, want 4", got)
	}
}

func TestResolverResolvesOrdinalAfterExtensionFilter(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "budget.xlsx", "extension": ".xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "readme.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "forecast.xls", "extension": "xls"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-4", "title": "actuals.xlsx", "extension": "xlsx"},
			},
		},
		Selectors: []Selector{{Type: "file", FileType: "excel", Ordinal: 2}},
	})

	assertResolvedAssetIDs(t, result, "file-3")
}

func TestResolverResolvesChineseOrdinalAfterExcelAliasFilter(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "budget.xlsx", "extension": ".xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "readme.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "forecast.xls", "extension": "xls"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-4", "title": "actuals.xlsx", "extension": "xlsx"},
			},
		},
		Selectors: []Selector{{Type: "file", Selector: "\u7b2c\u4e8c\u4e2a\u8868\u683c"}},
	})

	assertResolvedAssetIDs(t, result, "file-3")
}

func TestResolverResolvesLastPDF(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "first.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "sheet.xlsx", "extension": "xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "last.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{
			Type:        "file",
			Extension:   "pdf",
			OrdinalText: "last",
		}},
	})

	assertResolvedAssetIDs(t, result, "file-3")
}

func TestResolverResolvesLastPDFSelectorPhrase(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "first.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "sheet.xlsx", "extension": "xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "last.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{Type: "file", Selector: "last PDF"}},
	})

	assertResolvedAssetIDs(t, result, "file-3")
}

func TestResolverResolvesChineseLastPDFSelector(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "first.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "sheet.xlsx", "extension": "xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "last.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{Type: "file", Selector: "\u6700\u540e\u4e00\u4e2a PDF"}},
	})

	assertResolvedAssetIDs(t, result, "file-3")
}

func TestResolverReturnsAmbiguousForContainsMatch(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "invoice-january.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "invoice-february.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "notes.txt", "extension": "txt"},
			},
		},
		Selectors: []Selector{{Type: "file", TitleContains: "invoice"}},
	})

	if len(result.Resolutions) != 1 {
		t.Fatalf("resolutions len = %d, want 1", len(result.Resolutions))
	}
	resolution := result.Resolutions[0]
	if resolution.Status != StatusAmbiguous {
		t.Fatalf("status = %q, want %q", resolution.Status, StatusAmbiguous)
	}
	if got := len(resolution.Candidates); got != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", got, resolution.Candidates)
	}
	if len(result.Assets) != 0 {
		t.Fatalf("flattened assets = %#v, want empty", result.Assets)
	}
}

func TestResolverResolvesUniqueFuzzyName(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "quarterly-budget-final.xlsx", "extension": "xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "invoice.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{Type: "file", FuzzyName: "budget final"}},
	})

	assertResolvedAssetIDs(t, result, "file-1")
}

func TestResolverMergesSelectedIDsWithVisibleFileDetails(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"page": map[string]interface{}{
				"metadata": map[string]interface{}{
					"selected_file_ids": "file-2",
				},
			},
			"visible_files": []interface{}{
				map[string]interface{}{"file_id": "file-1", "name": "one.pdf"},
				map[string]interface{}{"file_id": "file-2", "name": "selected.xlsx", "workspace_id": "workspace-1"},
			},
		},
		Selectors: []Selector{{Type: "file"}},
	})

	assertResolvedAssetIDs(t, result, "file-2")
	asset := result.Assets[0]
	if asset.Name != "selected.xlsx" || asset.WorkspaceID != "workspace-1" {
		t.Fatalf("asset = %#v, want selected file details merged", asset)
	}
}

func TestResolverDirectIDMustMatchFilters(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "only.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{Type: "file", FileID: "file-1", FileType: "excel"}},
	})

	if got := result.Resolutions[0].Status; got != StatusNotFound {
		t.Fatalf("status = %q, want %q", got, StatusNotFound)
	}
}

func TestResolverReturnsNotFoundWhenFilteredOrdinalIsMissing(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "only.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{Type: "file", Extension: "pdf", Ordinal: 2}},
	})

	if len(result.Resolutions) != 1 {
		t.Fatalf("resolutions len = %d, want 1", len(result.Resolutions))
	}
	resolution := result.Resolutions[0]
	if resolution.Status != StatusNotFound {
		t.Fatalf("status = %q, want %q", resolution.Status, StatusNotFound)
	}
	if !strings.Contains(resolution.Reason, "out of range") {
		t.Fatalf("reason = %q, want out of range", resolution.Reason)
	}
}

func TestResolverReturnsUnsupportedForNonFileSelector(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "only.pdf", "extension": "pdf"},
			},
		},
		Selectors: []Selector{{ResourceType: "agent", ID: "agent-1"}},
	})

	if got := result.Resolutions[0].Status; got != StatusUnsupported {
		t.Fatalf("status = %q, want %q", got, StatusUnsupported)
	}
}

func TestResolverCandidateLimitAppliesToAmbiguousCandidates(t *testing.T) {
	result := Resolve(Request{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "invoice-1.pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "invoice-2.pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "invoice-3.pdf"},
			},
		},
		Selectors:      []Selector{{Type: "file", TitleContains: "invoice"}},
		CandidateLimit: 2,
	})

	resolution := result.Resolutions[0]
	if resolution.Status != StatusAmbiguous {
		t.Fatalf("status = %q, want %q", resolution.Status, StatusAmbiguous)
	}
	if got := len(resolution.Candidates); got != 2 {
		t.Fatalf("candidate count = %d, want 2", got)
	}
}

func assertResolvedAssetIDs(t *testing.T, result Result, want ...string) {
	t.Helper()
	if len(result.Resolutions) != 1 {
		t.Fatalf("resolutions len = %d, want 1", len(result.Resolutions))
	}
	resolution := result.Resolutions[0]
	if resolution.Status != StatusResolved {
		t.Fatalf("status = %q, want %q; reason=%q candidates=%#v", resolution.Status, StatusResolved, resolution.Reason, resolution.Candidates)
	}
	gotResolution := make([]string, 0, len(resolution.Assets))
	for _, asset := range resolution.Assets {
		gotResolution = append(gotResolution, asset.ID)
	}
	if got := strings.Join(gotResolution, ","); got != strings.Join(want, ",") {
		t.Fatalf("resolution asset IDs = %q, want %q", got, strings.Join(want, ","))
	}
	gotResult := make([]string, 0, len(result.Assets))
	for _, asset := range result.Assets {
		gotResult = append(gotResult, asset.ID)
	}
	if got := strings.Join(gotResult, ","); got != strings.Join(want, ",") {
		t.Fatalf("result asset IDs = %q, want %q", got, strings.Join(want, ","))
	}
}
