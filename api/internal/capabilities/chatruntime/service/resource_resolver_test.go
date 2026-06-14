package service

import (
	"strings"
	"testing"
)

func TestResourceResolverPrefersSelectedFile(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "first.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "chosen.pdf", "extension": "pdf", "selected": true},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file"}})

	assertResolvedFileIDs(t, result, "file-2")
	if got := result.Results[0].Resources[0].Metadata["selected"]; got != true {
		t.Fatalf("selected metadata = %#v, want true", got)
	}
}

func TestResourceResolverResolvesVisibleOrdinalOneBased(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"visible_files": []interface{}{
				map[string]interface{}{"file_id": "file-1", "name": "one.txt"},
				map[string]interface{}{"file_id": "file-2", "name": "two.txt"},
				map[string]interface{}{"file_id": "file-3", "name": "three.txt"},
				map[string]interface{}{"file_id": "file-4", "name": "four.txt"},
				map[string]interface{}{"file_id": "file-5", "name": "five.txt"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file", Selector: "visible_files[4]"}})

	assertResolvedFileIDs(t, result, "file-4")
	if got := result.Results[0].Resources[0].Metadata["visible_ordinal"]; got != 4 {
		t.Fatalf("visible_ordinal = %#v, want 4", got)
	}
}

func TestResourceResolverResolvesOrdinalAfterExtensionFilter(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "budget.xlsx", "extension": ".xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "readme.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "forecast.xls", "extension": "xls"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-4", "title": "actuals.xlsx", "extension": "xlsx"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file", FileType: "excel", Ordinal: 2}})

	assertResolvedFileIDs(t, result, "file-3")
}

func TestResourceResolverResolvesLastPDF(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "first.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "sheet.xlsx", "extension": "xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "last.pdf", "extension": "pdf"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{
		Type:        "file",
		Extension:   "pdf",
		OrdinalText: "last",
	}})

	assertResolvedFileIDs(t, result, "file-3")
}

func TestResourceResolverReturnsAmbiguousForContainsMatch(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "invoice-january.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "invoice-february.pdf", "extension": "pdf"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-3", "title": "notes.txt", "extension": "txt"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file", TitleContains: "invoice"}})

	if len(result.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(result.Results))
	}
	resolution := result.Results[0]
	if resolution.Status != ResourceResolutionStatusAmbiguous {
		t.Fatalf("status = %q, want %q", resolution.Status, ResourceResolutionStatusAmbiguous)
	}
	if got := len(resolution.Candidates); got != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", got, resolution.Candidates)
	}
	if len(result.FileIDs) != 0 || len(result.Resources) != 0 {
		t.Fatalf("flattened resolved resources = %#v/%#v, want empty", result.FileIDs, result.Resources)
	}
}

func TestResourceResolverResolvesUniqueFuzzyName(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "quarterly-budget-final.xlsx", "extension": "xlsx"},
				map[string]interface{}{"resource_type": "file", "resource_id": "file-2", "title": "invoice.pdf", "extension": "pdf"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file", FuzzyName: "budget final"}})

	assertResolvedFileIDs(t, result, "file-1")
}

func TestResourceResolverAutoResolvesExactlyOneVisibleCandidate(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "only.pdf", "extension": "pdf"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file"}})

	assertResolvedFileIDs(t, result, "file-1")
}

func TestResourceResolverReturnsNotFoundWhenFilteredOrdinalIsMissing(t *testing.T) {
	input := ResourceResolverInput{
		OperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"resource_type": "file", "resource_id": "file-1", "title": "only.pdf", "extension": "pdf"},
			},
		},
	}

	result := NewResourceResolver().Resolve(input, []PlannerResourceRef{{Type: "file", Extension: "pdf", Ordinal: 2}})

	if len(result.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(result.Results))
	}
	resolution := result.Results[0]
	if resolution.Status != ResourceResolutionStatusNotFound {
		t.Fatalf("status = %q, want %q", resolution.Status, ResourceResolutionStatusNotFound)
	}
	if !strings.Contains(resolution.Reason, "out of range") {
		t.Fatalf("reason = %q, want out of range", resolution.Reason)
	}
}

func assertResolvedFileIDs(t *testing.T, result ResourceResolverResult, want ...string) {
	t.Helper()
	if len(result.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(result.Results))
	}
	resolution := result.Results[0]
	if resolution.Status != ResourceResolutionStatusResolved {
		t.Fatalf("status = %q, want %q; reason=%q candidates=%#v", resolution.Status, ResourceResolutionStatusResolved, resolution.Reason, resolution.Candidates)
	}
	if got := strings.Join(resolution.FileIDs, ","); got != strings.Join(want, ",") {
		t.Fatalf("resolution file IDs = %q, want %q", got, strings.Join(want, ","))
	}
	if got := strings.Join(result.FileIDs, ","); got != strings.Join(want, ",") {
		t.Fatalf("result file IDs = %q, want %q", got, strings.Join(want, ","))
	}
}
