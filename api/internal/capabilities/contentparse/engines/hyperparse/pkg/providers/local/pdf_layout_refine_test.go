package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/hyperparse"
	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestRefineNativeFullDocumentLayoutBuildsVisualRows(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{
			"suggest_vlm": true,
		},
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "paragraph",
					"page_index": 1,
					"order":      1,
					"text":       "Patient: TEST, PAT I ENT Age: 14 DDS-14-10962",
					"bbox":       map[string]any{"left": 0.1, "top": 0.9, "right": 0.9, "bottom": 0.8},
				},
				{
					"type":       "table",
					"page_index": 1,
					"order":      2,
					"payload": map[string]any{
						"detection_mode": "geometry_token_v2",
						"cells": []map[string]any{
							{"row": 1, "col": 1, "text": "Patient:", "bbox": map[string]any{"left": 0.10, "top": 0.90, "right": 0.22, "bottom": 0.86}},
							{"row": 1, "col": 2, "text": "TEST, PAT I ENT", "bbox": map[string]any{"left": 0.23, "top": 0.90, "right": 0.44, "bottom": 0.86}},
							{"row": 1, "col": 3, "text": "Age:", "bbox": map[string]any{"left": 0.55, "top": 0.90, "right": 0.62, "bottom": 0.86}},
							{"row": 1, "col": 4, "text": "14", "bbox": map[string]any{"left": 0.63, "top": 0.90, "right": 0.68, "bottom": 0.86}},
							{"row": 2, "col": 1, "text": "DIAGNOSIS:", "bbox": map[string]any{"left": 0.10, "top": 0.72, "right": 0.25, "bottom": 0.68}},
							{"row": 3, "col": 1, "text": "RIGHT AR M SH AV E BI OPS Y", "bbox": map[string]any{"left": 0.10, "top": 0.66, "right": 0.55, "bottom": 0.62}},
							{"row": 4, "col": 1, "text": "Clinical:", "bbox": map[string]any{"left": 0.10, "top": 0.60, "right": 0.24, "bottom": 0.56}},
							{"row": 4, "col": 2, "text": "R/ O WAR T", "bbox": map[string]any{"left": 0.25, "top": 0.60, "right": 0.42, "bottom": 0.56}},
							{"row": 5, "col": 1, "text": "Microscopic:", "bbox": map[string]any{"left": 0.10, "top": 0.54, "right": 0.28, "bottom": 0.50}},
							{"row": 5, "col": 2, "text": "T I N E A", "bbox": map[string]any{"left": 0.29, "top": 0.54, "right": 0.42, "bottom": 0.50}},
							{"row": 6, "col": 1, "text": "Doctor:", "bbox": map[string]any{"left": 0.10, "top": 0.48, "right": 0.22, "bottom": 0.44}},
							{"row": 6, "col": 2, "text": "DPMG", "bbox": map[string]any{"left": 0.23, "top": 0.48, "right": 0.34, "bottom": 0.44}},
							{"row": 7, "col": 1, "text": "Sex:", "bbox": map[string]any{"left": 0.10, "top": 0.42, "right": 0.18, "bottom": 0.38}},
							{"row": 7, "col": 2, "text": "MALE", "bbox": map[string]any{"left": 0.19, "top": 0.42, "right": 0.30, "bottom": 0.38}},
						},
					},
				},
			},
		},
	}
	inspect := map[string]any{}

	refineNativeFullDocumentLayout(fullDoc, inspect)

	chunks := normalizeMapSlice(fullDoc["chunks"].(map[string]any)["items"])
	if got, want := len(chunks), 8; got != want {
		t.Fatalf("refined chunks=%d want=%d: %#v", got, want, chunks)
	}
	gotTexts := []string{
		chunks[0]["text"].(string),
		chunks[1]["text"].(string),
		chunks[2]["text"].(string),
		chunks[3]["text"].(string),
	}
	wantTexts := []string{
		"Patient: TEST, PATIENT",
		"Age: 14",
		"DIAGNOSIS:",
		"RIGHT ARM SHAVE BIOPSY",
	}
	for i := range wantTexts {
		if gotTexts[i] != wantTexts[i] {
			t.Fatalf("text[%d]=%q want=%q all=%#v", i, gotTexts[i], wantTexts[i], gotTexts)
		}
	}
	if inspect["local_layout_refinement"] == nil {
		t.Fatalf("inspect should expose local_layout_refinement")
	}
}

func TestRefineNativeFullDocumentLayoutKeepsUncoveredNativeText(t *testing.T) {
	fullDoc := map[string]any{
		"document": map[string]any{"suggest_vlm": true},
		"chunks": map[string]any{
			"items": []map[string]any{
				{
					"type":       "paragraph",
					"page_index": 1,
					"order":      10,
					"text":       "NOTE: Elastosis perforans serpiginosa presents as small papules and needs clinical correlation.",
					"bbox":       map[string]any{"left": 0.1, "top": 0.4, "right": 0.8, "bottom": 0.3},
				},
				{
					"type":       "table",
					"page_index": 1,
					"order":      2,
					"payload": map[string]any{
						"detection_mode": "geometry_token_v2",
						"cells": []map[string]any{
							{"row": 1, "col": 1, "text": "Patient:", "bbox": map[string]any{"left": 0.10, "top": 0.90, "right": 0.22, "bottom": 0.86}},
							{"row": 1, "col": 2, "text": "TEST, PAT I ENT", "bbox": map[string]any{"left": 0.23, "top": 0.90, "right": 0.44, "bottom": 0.86}},
							{"row": 2, "col": 1, "text": "Age:", "bbox": map[string]any{"left": 0.55, "top": 0.84, "right": 0.62, "bottom": 0.80}},
							{"row": 2, "col": 2, "text": "14", "bbox": map[string]any{"left": 0.63, "top": 0.84, "right": 0.68, "bottom": 0.80}},
							{"row": 3, "col": 1, "text": "Doctor:", "bbox": map[string]any{"left": 0.10, "top": 0.78, "right": 0.22, "bottom": 0.74}},
							{"row": 3, "col": 2, "text": "DPMG", "bbox": map[string]any{"left": 0.23, "top": 0.78, "right": 0.34, "bottom": 0.74}},
							{"row": 4, "col": 1, "text": "Sex:", "bbox": map[string]any{"left": 0.10, "top": 0.72, "right": 0.18, "bottom": 0.68}},
							{"row": 4, "col": 2, "text": "MALE", "bbox": map[string]any{"left": 0.19, "top": 0.72, "right": 0.30, "bottom": 0.68}},
							{"row": 5, "col": 1, "text": "Date Received:", "bbox": map[string]any{"left": 0.10, "top": 0.66, "right": 0.28, "bottom": 0.62}},
							{"row": 5, "col": 2, "text": "05/01/2014", "bbox": map[string]any{"left": 0.29, "top": 0.66, "right": 0.42, "bottom": 0.62}},
							{"row": 6, "col": 1, "text": "Clinical:", "bbox": map[string]any{"left": 0.10, "top": 0.60, "right": 0.24, "bottom": 0.56}},
							{"row": 6, "col": 2, "text": "R/ O WAR T", "bbox": map[string]any{"left": 0.25, "top": 0.60, "right": 0.42, "bottom": 0.56}},
							{"row": 7, "col": 1, "text": "Microscopic:", "bbox": map[string]any{"left": 0.10, "top": 0.54, "right": 0.28, "bottom": 0.50}},
							{"row": 7, "col": 2, "text": "T I N E A", "bbox": map[string]any{"left": 0.29, "top": 0.54, "right": 0.42, "bottom": 0.50}},
							{"row": 8, "col": 1, "text": "GROSS DESCRIPTION:", "bbox": map[string]any{"left": 0.10, "top": 0.48, "right": 0.34, "bottom": 0.44}},
						},
					},
				},
			},
		},
	}
	inspect := map[string]any{}

	refineNativeFullDocumentLayout(fullDoc, inspect)

	chunks := normalizeMapSlice(fullDoc["chunks"].(map[string]any)["items"])
	foundNote := false
	for _, chunk := range chunks {
		if strings.Contains(fmt.Sprint(chunk["text"]), "Elastosis perforans") {
			foundNote = true
			break
		}
	}
	if !foundNote {
		t.Fatalf("uncovered native NOTE paragraph should be retained: %#v", chunks)
	}
	stats := inspect["local_layout_refinement"].(map[string]any)
	if stats["retained_original"] != 1 {
		t.Fatalf("retained_original=%v want 1", stats["retained_original"])
	}
}

func TestLayoutTextCoverageRequiresLabelTokens(t *testing.T) {
	generated := []string{
		normalizeLayoutComparableText("Patient: TEST, PATIENT DDS-14-10962"),
		normalizeLayoutComparableText("Date Obtained: 04/30/2014 Date Received: 05/01/2014"),
		normalizeLayoutComparableText("DERMATOPATHOLOGY REPORT"),
	}
	if layoutTextCoveredByGenerated("Patient: Age: 14 (01/01/00)Pathology:", generated) {
		t.Fatalf("multi-label native header should not be considered covered when generated rows miss Age/Pathology labels")
	}
	if !layoutTextCoveredByGenerated("Patient: TEST, PATIENT DDS-14-10962", generated) {
		t.Fatalf("exact generated row should be considered covered")
	}
}

func TestRefineNativeFullDocumentLayoutLabReportKeepsBodyText(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "LabReport.pdf"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("LabReport.pdf fixture is not included in the in-repo runtime mirror")
		}
		t.Fatal(err)
	}
	fullDoc, err := hyperparse.InspectPDFFullDocument(context.Background(), hyperparse.InspectPDFOptions{
		Filename:   "LabReport.pdf",
		Data:       data,
		Mode:       "strict",
		NativeOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	inspect := map[string]any{}
	refineNativeFullDocumentLayout(fullDoc, inspect)

	chunksW := fullDoc["chunks"].(map[string]any)
	items := normalizeMapSlice(chunksW["items"])
	if len(items) < 35 {
		t.Fatalf("layout refinement dropped too many chunks: got=%d items=%#v", len(items), items)
	}
	var joined strings.Builder
	for _, item := range items {
		joined.WriteString(" ")
		joined.WriteString(fmt.Sprint(item["text"]))
	}
	text := joined.String()
	for _, want := range []string{"AGE:", "PATHOLOGY", "DDS-14-10962", "DIAGNOSIS", "NOTE", "MICROSCOPIC DESCRIPTION"} {
		if !strings.Contains(strings.ToUpper(text), want) {
			t.Fatalf("refined LabReport missing %q in text: %s", want, text)
		}
	}
	stats := inspect["local_layout_refinement"].(map[string]any)
	if retained := intAny(stats["retained_original"]); retained == 0 {
		t.Fatalf("expected retained original chunks in LabReport refinement, stats=%#v", stats)
	}
}

func TestParsePDFLabReportKeepsMinerUComparableHeaderFields(t *testing.T) {
	t.Setenv("LOCAL_VLM_FALLBACK", "disabled")
	t.Setenv("LOCAL_VLM_SIDEBAR_RECOVERY", "disabled")
	t.Setenv("DOCSTILL_VLM_IMAGE_CAPTION", "0")
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "LabReport.pdf"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("LabReport.pdf fixture is not included in the in-repo runtime mirror")
		}
		t.Fatal(err)
	}

	doc, err := New().ParseBytes(context.Background(), "LabReport.pdf", data, extractcommon.ParseOptions{Mode: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	joined.WriteString(doc.Markdown)
	for _, chunk := range doc.Chunks {
		joined.WriteString(" ")
		joined.WriteString(chunk.Text)
		joined.WriteString(" ")
		joined.WriteString(chunk.Markdown)
	}
	text := strings.ToUpper(joined.String())
	for _, want := range []string{"AGE:", "PATHOLOGY", "DDS-14-10962"} {
		if !strings.Contains(text, want) {
			t.Fatalf("parsed LabReport missing MinerU-comparable header field %q in text: %s", want, text)
		}
	}
}
