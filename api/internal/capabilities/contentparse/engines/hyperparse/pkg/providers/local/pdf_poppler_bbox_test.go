package local

import (
	"context"
	"strings"
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestPreparePopplerPageNormalizesTextAndImageBoxes(t *testing.T) {
	page := popplerXMLPage{
		Number: 1,
		Width:  918,
		Height: 1188,
		Texts: []popplerXMLText{
			{Top: 417, Left: 65, Width: 126, Height: 18, Content: "<b>DIAGNOSIS:  </b>"},
		},
		Images: []popplerXMLImage{
			{Top: 672, Left: 121, Width: 288, Height: 216},
		},
	}

	preparePopplerPage(&page)

	if len(page.Lines) != 1 || page.Lines[0].Text != "DIAGNOSIS:" {
		t.Fatalf("unexpected lines=%#v", page.Lines)
	}
	if len(page.Figs) != 1 {
		t.Fatalf("figs=%d", len(page.Figs))
	}
	fig := page.Figs[0].Box
	if !near(fig.Left, 0.131808) || !near(fig.Top, 0.565657) || !near(fig.Right, 0.445534) || !near(fig.Bottom, 0.747475) {
		t.Fatalf("unexpected image bbox=%+v", fig)
	}
}

func TestApplyPopplerBBoxRefinementSkipsLargeDocumentsByDefault(t *testing.T) {
	t.Setenv("LOCAL_POPPLER_BBOX_MAX_PAGES", "2")
	t.Setenv("LOCAL_POPPLER_BBOX_LONG_DOC", "")

	doc := &extractcommon.DocumentResult{
		PageCount: 3,
		Chunks: []extractcommon.Chunk{{
			ID:        "c-1",
			Type:      "text",
			Text:      "hello",
			Page:      0,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.1, Top: 0.1, Right: 0.2, Bottom: 0.2},
		}},
	}

	stats := applyPopplerBBoxRefinement(context.Background(), "large.pdf", []byte("%PDF-1.4"), doc)
	if stats.applied {
		t.Fatalf("expected large document poppler refinement to be skipped")
	}
	if !strings.Contains(stats.warning, "skipped_large_document") {
		t.Fatalf("unexpected warning: %q", stats.warning)
	}
}

func TestPreparePopplerPageOrdersSameVisualRowByLeft(t *testing.T) {
	page := popplerXMLPage{
		Number: 1,
		Width:  918,
		Height: 1188,
		Texts: []popplerXMLText{
			{Top: 159, Left: 134, Width: 142, Height: 24, Content: "<b>TEST, PATIENT</b>"},
			{Top: 162, Left: 65, Width: 50, Height: 14, Content: "Patient:"},
			{Top: 159, Left: 485, Width: 84, Height: 17, Content: "14 (01/01/00)"},
			{Top: 162, Left: 431, Width: 33, Height: 14, Content: "Age:"},
			{Top: 195, Left: 485, Width: 37, Height: 17, Content: "MALE"},
			{Top: 198, Left: 431, Width: 30, Height: 14, Content: "Sex:"},
		},
	}

	preparePopplerPage(&page)

	got := make([]string, 0, len(page.Lines))
	for _, line := range page.Lines {
		got = append(got, line.Text)
	}
	want := []string{"Patient:", "TEST, PATIENT", "Age:", "14 (01/01/00)", "Sex:", "MALE"}
	if len(got) != len(want) {
		t.Fatalf("got lines=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d got %q want %q; all=%v", i, got[i], want[i], got)
		}
	}
}

func TestAssignPopplerTextBoxUsesLabelAndValueForShortField(t *testing.T) {
	page := popplerXMLPage{
		Lines: []popplerTextLine{
			{Text: "Acct#:", Box: extractcommon.BBox{Left: 0.070, Top: 0.167, Right: 0.114, Bottom: 0.178}},
			{Text: "Sex:", Box: extractcommon.BBox{Left: 0.469, Top: 0.167, Right: 0.502, Bottom: 0.178}},
			{Text: "MALE", Box: extractcommon.BBox{Left: 0.528, Top: 0.164, Right: 0.568, Bottom: 0.178}},
			{Text: "DPMG use only:", Box: extractcommon.BBox{Left: 0.675, Top: 0.167, Right: 0.779, Bottom: 0.178}},
		},
	}
	ch := extractcommon.Chunk{
		Type:      "text",
		Text:      "Sex: MALE",
		Precision: "reliable",
		BBox:      &extractcommon.BBox{Left: 0.114, Top: 0.165, Right: 0.164, Bottom: 0.178},
	}

	if !assignPopplerTextBox(&ch, &page) {
		t.Fatal("expected poppler bbox assignment")
	}
	if ch.BBox == nil || !near(ch.BBox.Left, 0.469) || !near(ch.BBox.Right, 0.568) {
		t.Fatalf("unexpected sex bbox=%+v", ch.BBox)
	}
}

func TestAssignPopplerTextBoxExpandsLeftFieldLabel(t *testing.T) {
	page := popplerXMLPage{
		Lines: []popplerTextLine{
			{Text: "Pathology #:", Box: extractcommon.BBox{Left: 0.692, Top: 0.134, Right: 0.779, Bottom: 0.151}},
			{Text: "DDS-14-10962", Box: extractcommon.BBox{Left: 0.793, Top: 0.134, Right: 0.924, Bottom: 0.154}},
		},
	}
	ch := extractcommon.Chunk{
		Type:      "text",
		Text:      "DDS-14-10962",
		Precision: "reliable",
		BBox:      &extractcommon.BBox{Left: 0.793, Top: 0.134, Right: 0.924, Bottom: 0.154},
	}

	if !assignPopplerTextBox(&ch, &page) {
		t.Fatal("expected poppler bbox assignment")
	}
	if ch.BBox == nil || !near(ch.BBox.Left, 0.692) || ch.Text != "Pathology#: DDS-14-10962" {
		t.Fatalf("unexpected chunk=%+v", ch)
	}
}

func TestAssignPopplerTextBoxDoesNotExpandSectionHeadingLabel(t *testing.T) {
	page := popplerXMLPage{
		Lines: []popplerTextLine{
			{Text: "GROSS DESCRIPTION:", Box: extractcommon.BBox{Left: 0.070, Top: 0.256, Right: 0.267, Bottom: 0.269}},
			{Text: "TMA:kg", Box: extractcommon.BBox{Left: 0.267, Top: 0.256, Right: 0.332, Bottom: 0.269}},
		},
	}
	ch := extractcommon.Chunk{
		Type:      "text",
		Text:      "TMA:kg",
		Precision: "reliable",
		BBox:      &extractcommon.BBox{Left: 0.267, Top: 0.256, Right: 0.332, Bottom: 0.269},
	}

	if !assignPopplerTextBox(&ch, &page) {
		t.Fatal("expected poppler bbox assignment")
	}
	if ch.BBox == nil || !near(ch.BBox.Left, 0.267) || ch.Text != "TMA: kg" {
		t.Fatalf("unexpected chunk=%+v", ch)
	}
}

func TestAssignPopplerTextBoxRepairsNoisyLocalText(t *testing.T) {
	page := popplerXMLPage{
		Lines: []popplerTextLine{
			{Text: "DIAGNOSIS:", Box: extractcommon.BBox{Left: 0.070, Top: 0.351, Right: 0.197, Bottom: 0.366}},
			{Text: "A. SKIN, RIGHT ARM, SHAVE BIOPSY:", Box: extractcommon.BBox{Left: 0.129, Top: 0.387, Right: 0.553, Bottom: 0.402}},
			{Text: "COMPATIBLE WITH PERFORATING DISORDER WITH FEATURES OF", Box: extractcommon.BBox{Left: 0.188, Top: 0.406, Right: 0.954, Bottom: 0.421}},
			{Text: "ELASTOSIS PERFORANS SERPIGINOSA.", Box: extractcommon.BBox{Left: 0.188, Top: 0.425, Right: 0.599, Bottom: 0.440}},
		},
	}
	ch := extractcommon.Chunk{
		Type:      "text",
		Text:      "SKIN, RI A. GHT ARM, SHAVE BIOPSY: COMPATIBLE WI TH PERFORATING DI SORDER WITH FEATURES OF ELASTOSIS PERFORANS SERPIGI B.",
		Precision: "reliable",
		BBox:      &extractcommon.BBox{Left: 0.127, Top: 0.237, Right: 1.0, Bottom: 0.437},
	}

	if !assignPopplerTextBox(&ch, &page) {
		t.Fatal("expected poppler bbox assignment")
	}
	if ch.BBox == nil || !near(ch.BBox.Top, 0.387) || !near(ch.BBox.Bottom, 0.440) || !near(ch.BBox.Right, 0.954) {
		t.Fatalf("unexpected repaired bbox=%+v", ch.BBox)
	}
	if ch.Text != "A. SKIN, RIGHT ARM, SHAVE BIOPSY: COMPATIBLE WITH PERFORATING DISORDER WITH FEATURES OF ELASTOSIS PERFORANS SERPIGINOSA." {
		t.Fatalf("unexpected repaired text=%q", ch.Text)
	}
}

func TestAssignPopplerTextBoxIncludesLeadingListMarker(t *testing.T) {
	page := popplerXMLPage{
		Lines: []popplerTextLine{
			{Text: "A.", Box: extractcommon.BBox{Left: 0.129, Top: 0.387, Right: 0.156, Bottom: 0.402}},
			{Text: "SKIN, RIGHT ARM, SHAVE BIOPSY:", Box: extractcommon.BBox{Left: 0.188, Top: 0.387, Right: 0.553, Bottom: 0.402}},
			{Text: "COMPATIBLE WITH PERFORATING DISORDER WITH FEATURES OF", Box: extractcommon.BBox{Left: 0.188, Top: 0.406, Right: 0.954, Bottom: 0.421}},
		},
	}
	ch := extractcommon.Chunk{
		Type:      "text",
		Text:      "SKIN, RIGHT ARM, SHAVE BIOPSY: COMPATIBLE WITH PERFORATING DISORDER WITH FEATURES OF",
		Precision: "reliable",
		BBox:      &extractcommon.BBox{Left: 0.188, Top: 0.387, Right: 0.954, Bottom: 0.421},
	}

	if !assignPopplerTextBox(&ch, &page) {
		t.Fatal("expected poppler bbox assignment")
	}
	if ch.BBox == nil || !near(ch.BBox.Left, 0.129) || !near(ch.BBox.Right, 0.954) {
		t.Fatalf("unexpected list bbox=%+v", ch.BBox)
	}
}

func TestRepairChunkPagesWithPopplerMovesLongUniqueText(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{
				ID:        "gross-a",
				Type:      "text",
				Text:      `A. Received in formalin in a container labeled with the patient's name and "R arm" is a single specimen.`,
				Page:      0,
				Ordinal:   1,
				Precision: "reliable",
				BBox:      &extractcommon.BBox{Left: 0.12, Top: 0.38, Right: 0.95, Bottom: 0.43},
			},
		},
	}
	pages := []popplerXMLPage{
		{
			Lines: []popplerTextLine{
				{Text: "A. SKIN, RIGHT ARM, SHAVE BIOPSY:", Box: extractcommon.BBox{Left: 0.12, Top: 0.38, Right: 0.55, Bottom: 0.40}},
				{Text: "COMPATIBLE WITH PERFORATING DISORDER", Box: extractcommon.BBox{Left: 0.18, Top: 0.41, Right: 0.95, Bottom: 0.43}},
			},
		},
		{
			Lines: []popplerTextLine{
				{Text: `A. Received in formalin in a container labeled with the patient's name and "R arm"`, Box: extractcommon.BBox{Left: 0.07, Top: 0.28, Right: 0.94, Bottom: 0.30}},
				{Text: "is a single specimen.", Box: extractcommon.BBox{Left: 0.10, Top: 0.30, Right: 0.40, Bottom: 0.32}},
			},
		},
	}

	if repaired := repairChunkPagesWithPoppler(doc, pages); repaired != 1 {
		t.Fatalf("repaired=%d", repaired)
	}
	if doc.Chunks[0].Page != 1 {
		t.Fatalf("page=%d want 1", doc.Chunks[0].Page)
	}
	if doc.Chunks[0].BBox != nil || doc.Chunks[0].Precision != "" {
		t.Fatalf("expected bbox/precision reset for reassignment, chunk=%+v", doc.Chunks[0])
	}
}

func TestRepairChunkPagesWithPopplerDoesNotMoveRepeatedShortHeader(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{{
			ID:        "header",
			Type:      "text",
			Text:      "DPMG use only: 0700800",
			Page:      0,
			Ordinal:   1,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.67, Top: 0.16, Right: 0.88, Bottom: 0.18},
		}},
	}
	pages := []popplerXMLPage{
		{Lines: []popplerTextLine{{Text: "DPMG use only: 0700800", Box: extractcommon.BBox{Left: 0.67, Top: 0.16, Right: 0.88, Bottom: 0.18}}}},
		{Lines: []popplerTextLine{{Text: "DPMG use only: 0700800", Box: extractcommon.BBox{Left: 0.67, Top: 0.16, Right: 0.88, Bottom: 0.18}}}},
	}

	if repaired := repairChunkPagesWithPoppler(doc, pages); repaired != 0 {
		t.Fatalf("repaired=%d", repaired)
	}
	if doc.Chunks[0].Page != 0 {
		t.Fatalf("page=%d want 0", doc.Chunks[0].Page)
	}
}

func TestRepairContextualPageIslandsMovesRepeatedHeaderRun(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{ID: "age", Type: "text", Text: "Age: 14 (01/01/00)", Page: 1, Ordinal: 1},
			{ID: "dpmg", Type: "text", Text: "DPMG use only: 0700800", Page: 0, Ordinal: 2, Precision: "reliable", BBox: &extractcommon.BBox{Left: 0.67, Top: 0.16, Right: 0.88, Bottom: 0.18}},
			{ID: "addr1", Type: "text", Text: "3301 C st Suite 200E", Page: 0, Ordinal: 3, Precision: "reliable", BBox: &extractcommon.BBox{Left: 0.14, Top: 0.20, Right: 0.28, Bottom: 0.22}},
			{ID: "addr2", Type: "text", Text: "Sacramento CA, 95816", Page: 0, Ordinal: 4, Precision: "reliable", BBox: &extractcommon.BBox{Left: 0.14, Top: 0.22, Right: 0.29, Bottom: 0.24}},
			{ID: "dates", Type: "text", Text: "Date Obtained: 04/30/2014 Date Received: 05/01/2014", Page: 1, Ordinal: 5},
		},
	}

	if repaired := repairContextualPageIslands(doc); repaired != 3 {
		t.Fatalf("repaired=%d", repaired)
	}
	for i := 1; i <= 3; i++ {
		if doc.Chunks[i].Page != 1 {
			t.Fatalf("chunk %d page=%d want 1", i, doc.Chunks[i].Page)
		}
		if doc.Chunks[i].BBox != nil || doc.Chunks[i].Precision != "" {
			t.Fatalf("chunk %d should reset bbox/precision: %+v", i, doc.Chunks[i])
		}
	}
}

func TestSplitHeaderChunksWithPopplerSeparatesTitleAndAddress(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{{
			ID:        "header",
			Type:      "text",
			Text:      "DERMATOPATHOLOGY REPORT 3301 C Street, Ste 200E Sacramento,CA 95816",
			Page:      0,
			Ordinal:   1,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.38, Top: 0.035, Right: 0.93, Bottom: 0.105},
		}},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "DERMATOPATHOLOGY", Box: extractcommon.BBox{Left: 0.386, Top: 0.047, Right: 0.660, Bottom: 0.063}},
			{Text: "REPORT", Box: extractcommon.BBox{Left: 0.475, Top: 0.068, Right: 0.570, Bottom: 0.084}},
			{Text: "3301 C Street, Ste 200E", Box: extractcommon.BBox{Left: 0.778, Top: 0.038, Right: 0.922, Bottom: 0.048}},
			{Text: "Sacramento,CA 95816", Box: extractcommon.BBox{Left: 0.778, Top: 0.052, Right: 0.915, Bottom: 0.062}},
		},
	}}

	if splits := splitHeaderChunksWithPoppler(doc, pages); splits != 1 {
		t.Fatalf("splits=%d", splits)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Type != "heading" || doc.Chunks[0].Text != "DERMATOPATHOLOGY REPORT" {
		t.Fatalf("unexpected title chunk=%+v", doc.Chunks[0])
	}
	if doc.Chunks[1].Type != "marginalia" || doc.Chunks[1].Text != "3301 C Street, Ste 200E Sacramento,CA 95816" {
		t.Fatalf("unexpected address chunk=%+v", doc.Chunks[1])
	}
}

func TestMergeKnownFormFieldsWithPoppler(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{ID: "doctor", Type: "text", Text: "Doctor:", Page: 0, Ordinal: 1, BBox: &extractcommon.BBox{Left: 0.070, Top: 0.192, Right: 0.125, Bottom: 0.204}},
			{ID: "doctor-value", Type: "text", Text: "DPMG", Page: 0, Ordinal: 2, BBox: &extractcommon.BBox{Left: 0.136, Top: 0.190, Right: 0.306, Bottom: 0.206}},
			{ID: "date-value", Type: "text", Text: "04/30/2014", Page: 0, Ordinal: 3, BBox: &extractcommon.BBox{Left: 0.793, Top: 0.192, Right: 0.879, Bottom: 0.205}},
			{ID: "date-received", Type: "text", Text: "Date Received: 05/01/2014", Page: 0, Ordinal: 4, BBox: &extractcommon.BBox{Left: 0.675, Top: 0.205, Right: 0.879, Bottom: 0.221}},
		},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "Doctor:", Box: extractcommon.BBox{Left: 0.071, Top: 0.192, Right: 0.125, Bottom: 0.204}},
			{Text: "DPMG", Box: extractcommon.BBox{Left: 0.146, Top: 0.189, Right: 0.190, Bottom: 0.205}},
			{Text: "3301 C st Suite 200E", Box: extractcommon.BBox{Left: 0.146, Top: 0.203, Right: 0.277, Bottom: 0.219}},
			{Text: "Sacramento CA, 95816", Box: extractcommon.BBox{Left: 0.146, Top: 0.218, Right: 0.289, Bottom: 0.234}},
			{Text: "Date Obtained:", Box: extractcommon.BBox{Left: 0.675, Top: 0.192, Right: 0.779, Bottom: 0.204}},
			{Text: "04/30/2014", Box: extractcommon.BBox{Left: 0.793, Top: 0.192, Right: 0.879, Bottom: 0.205}},
			{Text: "Date Received", Box: extractcommon.BBox{Left: 0.675, Top: 0.205, Right: 0.771, Bottom: 0.221}},
			{Text: ":", Box: extractcommon.BBox{Left: 0.771, Top: 0.205, Right: 0.779, Bottom: 0.221}},
			{Text: "05/01/2014", Box: extractcommon.BBox{Left: 0.793, Top: 0.205, Right: 0.879, Bottom: 0.221}},
		},
	}}

	if merges := mergeKnownFormFieldsWithPoppler(doc, pages); merges != 2 {
		t.Fatalf("merges=%d", merges)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Text != "Doctor: DPMG 3301 C st Suite 200E Sacramento CA, 95816" {
		t.Fatalf("unexpected doctor=%q", doc.Chunks[0].Text)
	}
	if doc.Chunks[1].Text != "Date Obtained: 04/30/2014 Date Received: 05/01/2014" {
		t.Fatalf("unexpected dates=%q", doc.Chunks[1].Text)
	}
}

func TestMergeInlineHeadingContinuations(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{ID: "gross", Type: "heading", Text: "GROSS DESCRIPTION:", Page: 0, Ordinal: 1, BBox: &extractcommon.BBox{Left: 0.07, Top: 0.256, Right: 0.267, Bottom: 0.269}},
			{ID: "tma", Type: "text", Text: "TMA: kg", Page: 0, Ordinal: 2, BBox: &extractcommon.BBox{Left: 0.267, Top: 0.256, Right: 0.332, Bottom: 0.269}},
		},
	}

	if merges := mergeInlineHeadingContinuations(doc); merges != 1 {
		t.Fatalf("merges=%d", merges)
	}
	if len(doc.Chunks) != 1 || doc.Chunks[0].Type != "heading" || doc.Chunks[0].Text != "GROSS DESCRIPTION: TMA: kg" {
		t.Fatalf("unexpected chunks=%+v", doc.Chunks)
	}
}

func TestMergeAdjacentParagraphContinuations(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{ID: "a", Type: "text", Text: "This report may include a photomicrograph.", Page: 0, Ordinal: 1, BBox: &extractcommon.BBox{Left: 0.07, Top: 0.470, Right: 0.94, Bottom: 0.477}},
			{ID: "b", Type: "text", Text: "that you view.", Page: 0, Ordinal: 2, BBox: &extractcommon.BBox{Left: 0.07, Top: 0.479, Right: 0.88, Bottom: 0.486}},
			{ID: "c", Type: "text", Text: "A. Next list item", Page: 0, Ordinal: 3, BBox: &extractcommon.BBox{Left: 0.07, Top: 0.502, Right: 0.88, Bottom: 0.512}},
		},
	}

	if merges := mergeAdjacentParagraphContinuations(doc); merges != 1 {
		t.Fatalf("merges=%d", merges)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Text != "This report may include a photomicrograph. that you view." {
		t.Fatalf("unexpected merged text=%q", doc.Chunks[0].Text)
	}
}

func TestReplaceFooterChunksWithPopplerBuildsColumns(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{
			{ID: "page", Type: "marginalia", Text: "Page 1 of 2", Page: 0, Ordinal: 1, BBox: &extractcommon.BBox{Left: 0.84, Top: 0.92, Right: 0.94, Bottom: 0.94}},
			{ID: "footer-old", Type: "marginalia", Text: "Robert W. Ghiselli David R. Guillén", Page: 0, Ordinal: 2, BBox: &extractcommon.BBox{Left: 0.08, Top: 0.947, Right: 0.41, Bottom: 0.966}},
		},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "Robert W. Ghiselli, M.D.", Box: extractcommon.BBox{Left: 0.08, Top: 0.947, Right: 0.23, Bottom: 0.957}},
			{Text: "Board Certified in Dermatopathology", Box: extractcommon.BBox{Left: 0.08, Top: 0.960, Right: 0.23, Bottom: 0.967}},
			{Text: "David R. Guillén, M.D.", Box: extractcommon.BBox{Left: 0.26, Top: 0.947, Right: 0.40, Bottom: 0.957}},
			{Text: "Board Certified in Dermatopathology", Box: extractcommon.BBox{Left: 0.26, Top: 0.960, Right: 0.40, Bottom: 0.967}},
		},
	}}

	if updated := replaceFooterChunksWithPoppler(doc, pages); updated != 1 {
		t.Fatalf("updated=%d", updated)
	}
	if len(doc.Chunks) != 3 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[1].Text != "Robert W. Ghiselli, M.D. Board Certified in Dermatopathology" {
		t.Fatalf("unexpected first footer=%q", doc.Chunks[1].Text)
	}
	if doc.Chunks[2].Text != "David R. Guillén, M.D. Board Certified in Dermatopathology" {
		t.Fatalf("unexpected second footer=%q", doc.Chunks[2].Text)
	}
}

func TestSplitStructuredTextChunksByListMarkers(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{{
			ID:        "diagnosis-b",
			Type:      "text",
			Text:      "B section with two numbered findings",
			Page:      0,
			Ordinal:   1,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.12, Top: 0.45, Right: 0.96, Bottom: 0.56},
		}},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "B.", Box: extractcommon.BBox{Left: 0.129, Top: 0.463, Right: 0.154, Bottom: 0.478}},
			{Text: "SKIN, LEFT NECK, SHAVE BIOPSY:", Box: extractcommon.BBox{Left: 0.188, Top: 0.463, Right: 0.553, Bottom: 0.478}},
			{Text: "1.", Box: extractcommon.BBox{Left: 0.188, Top: 0.482, Right: 0.214, Bottom: 0.497}},
			{Text: "COMPATIBLE WITH PERFORATING DISORDER", Box: extractcommon.BBox{Left: 0.247, Top: 0.482, Right: 0.93, Bottom: 0.497}},
			{Text: "2.", Box: extractcommon.BBox{Left: 0.188, Top: 0.520, Right: 0.214, Bottom: 0.535}},
			{Text: "ASSOCIATED SPONGIOTIC DERMATITIS", Box: extractcommon.BBox{Left: 0.247, Top: 0.520, Right: 0.94, Bottom: 0.535}},
		},
	}}

	if splits := splitStructuredTextChunksWithPoppler(doc, pages); splits != 2 {
		t.Fatalf("splits=%d", splits)
	}
	if len(doc.Chunks) != 3 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Text != "B. SKIN, LEFT NECK, SHAVE BIOPSY:" {
		t.Fatalf("unexpected first text=%q", doc.Chunks[0].Text)
	}
	if doc.Chunks[1].Text != "1. COMPATIBLE WITH PERFORATING DISORDER" {
		t.Fatalf("unexpected second text=%q", doc.Chunks[1].Text)
	}
	if doc.Chunks[2].BBox == nil || !near(doc.Chunks[2].BBox.Left, 0.188) {
		t.Fatalf("unexpected third bbox=%+v", doc.Chunks[2].BBox)
	}
}

func TestSplitStructuredTextChunksByInlineListMarkers(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{{
			ID:        "gross",
			Type:      "text",
			Text:      "A and B gross description",
			Page:      0,
			Ordinal:   1,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.06, Top: 0.28, Right: 0.95, Bottom: 0.36},
		}},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "A. Received in formalin", Box: extractcommon.BBox{Left: 0.070, Top: 0.286, Right: 0.94, Bottom: 0.298}},
			{Text: "continued cassette A.", Box: extractcommon.BBox{Left: 0.100, Top: 0.302, Right: 0.90, Bottom: 0.314}},
			{Text: "B. Received in formalin", Box: extractcommon.BBox{Left: 0.070, Top: 0.330, Right: 0.94, Bottom: 0.342}},
			{Text: "continued cassette B.", Box: extractcommon.BBox{Left: 0.100, Top: 0.346, Right: 0.90, Bottom: 0.358}},
		},
	}}

	if splits := splitStructuredTextChunksWithPoppler(doc, pages); splits != 1 {
		t.Fatalf("splits=%d", splits)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Text != "A. Received in formalin continued cassette A." {
		t.Fatalf("unexpected first text=%q", doc.Chunks[0].Text)
	}
	if doc.Chunks[1].Text != "B. Received in formalin continued cassette B." {
		t.Fatalf("unexpected second text=%q", doc.Chunks[1].Text)
	}
}

func TestSplitStructuredTextChunksSeparatesLeadingHeading(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{{
			ID:        "micro",
			Type:      "text",
			Text:      "MICROSCOPIC DESCRIPTION: The sections show findings.",
			Page:      0,
			Ordinal:   1,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.07, Top: 0.37, Right: 0.95, Bottom: 0.45},
		}},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "MICROSCOPIC DESCRIPTION:", Box: extractcommon.BBox{Left: 0.07, Top: 0.374, Right: 0.34, Bottom: 0.389}},
			{Text: "The sections show findings.", Box: extractcommon.BBox{Left: 0.07, Top: 0.402, Right: 0.63, Bottom: 0.418}},
		},
	}}

	if splits := splitStructuredTextChunksWithPoppler(doc, pages); splits != 1 {
		t.Fatalf("splits=%d", splits)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Type != "heading" || doc.Chunks[0].Text != "MICROSCOPIC DESCRIPTION:" {
		t.Fatalf("unexpected heading=%+v", doc.Chunks[0])
	}
	if doc.Chunks[1].Type != "text" || doc.Chunks[1].Text != "The sections show findings." {
		t.Fatalf("unexpected body=%+v", doc.Chunks[1])
	}
}

func TestAssignPopplerImageBoxFillsFigureBBox(t *testing.T) {
	page := popplerXMLPage{
		Figs: []popplerImageItem{{Box: extractcommon.BBox{Left: 0.13, Top: 0.56, Right: 0.45, Bottom: 0.75}}},
	}
	ch := extractcommon.Chunk{Type: "figure", Precision: "unreliable"}

	if !assignPopplerImageBox(&ch, &page) {
		t.Fatal("expected figure bbox assignment")
	}
	if ch.Precision != "reliable" || ch.BBox == nil || ch.BBox.Left != 0.13 {
		t.Fatalf("chunk=%+v", ch)
	}
}

func TestSplitSparseCaptionChunksWithPoppler(t *testing.T) {
	doc := &extractcommon.DocumentResult{
		Chunks: []extractcommon.Chunk{{
			ID:        "caption",
			Type:      "text",
			Text:      "A B",
			Page:      0,
			Ordinal:   1,
			Precision: "reliable",
			BBox:      &extractcommon.BBox{Left: 0.42, Top: 0.73, Right: 0.89, Bottom: 0.76},
		}},
	}
	pages := []popplerXMLPage{{
		Lines: []popplerTextLine{
			{Text: "A", Box: extractcommon.BBox{Left: 0.278, Top: 0.751, Right: 0.291, Bottom: 0.763}},
			{Text: "B", Box: extractcommon.BBox{Left: 0.722, Top: 0.751, Right: 0.735, Bottom: 0.763}},
		},
	}}

	if splits := splitSparseCaptionChunksWithPoppler(doc, pages); splits != 1 {
		t.Fatalf("splits=%d", splits)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Text != "A" || doc.Chunks[1].Text != "B" {
		t.Fatalf("unexpected texts=%q/%q", doc.Chunks[0].Text, doc.Chunks[1].Text)
	}
	if doc.Chunks[0].BBox == nil || !near(doc.Chunks[0].BBox.Left, 0.278) {
		t.Fatalf("unexpected first bbox=%+v", doc.Chunks[0].BBox)
	}
	if doc.Chunks[1].Ordinal != 2 || doc.Chunks[1].Type != "marginalia" {
		t.Fatalf("unexpected second chunk=%+v", doc.Chunks[1])
	}
}

func near(a, b float64) bool {
	if a > b {
		return a-b < 0.0005
	}
	return b-a < 0.0005
}
