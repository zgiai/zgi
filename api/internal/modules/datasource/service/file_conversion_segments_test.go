package service

import (
	"fmt"
	"strings"
	"testing"
)

func TestSplitFileConversionContentBatchesMarkdownTableRows(t *testing.T) {
	var content strings.Builder
	content.WriteString("| name | amount |\n| --- | --- |\n")
	for i := 1; i <= 85; i++ {
		fmt.Fprintf(&content, "| customer-%d | %d |\n", i, i*10)
	}

	segments, tabular, err := splitFileConversionContent(content.String())
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if !tabular {
		t.Fatal("splitFileConversionContent() tabular = false, want true")
	}
	if got, want := len(segments), 3; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
	if got, want := len(segments[0].SourceRowIndexes), 40; got != want {
		t.Fatalf("first segment rows = %d, want %d", got, want)
	}
	if got, want := segments[2].SourceRowIndexes, []int{81, 82, 83, 84, 85}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("last segment indexes = %v, want %v", got, want)
	}
	if !strings.Contains(segments[2].Content, "SOURCE_ROW_85: | customer-85 | 850 |") {
		t.Fatalf("last segment content does not preserve source row: %q", segments[2].Content)
	}
}

func TestSplitFileConversionContentRecognizesCSVRows(t *testing.T) {
	segments, tabular, err := splitFileConversionContent("name,amount\nAlice,10\nBob,20\n")
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if !tabular {
		t.Fatal("splitFileConversionContent() tabular = false, want true")
	}
	if got, want := segments[0].SourceRowIndexes, []int{1, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("source row indexes = %v, want %v", got, want)
	}
}

func TestSplitFileConversionContentPreservesMarkdownTextAndAllTables(t *testing.T) {
	content := `Introduction

| name | amount |
| --- | --- |
| Alice | 10 |

Between tables

| sku | quantity |
| --- | --- |
| A-1 | 2 |

Conclusion`

	segments, _, err := splitFileConversionContent(content)
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if got, want := len(segments), 3; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
	wants := []struct {
		content string
		tabular bool
	}{
		{content: "Introduction\n\nBetween tables\n\nConclusion", tabular: false},
		{content: "Source table header: | name | amount |", tabular: true},
		{content: "Source table header: | sku | quantity |", tabular: true},
	}
	for i, want := range wants {
		if segments[i].Tabular != want.tabular || !strings.Contains(segments[i].Content, want.content) {
			t.Fatalf("segment %d = %#v, want tabular=%v containing %q", i, segments[i], want.tabular, want.content)
		}
	}
}

func TestSplitFileConversionContentPreservesHTMLTextAndAllTables(t *testing.T) {
	content := `<p>Introduction</p>
<table><tr><th>name</th><th>amount</th></tr><tr><td>Alice</td><td>10</td></tr></table>
<p>Between tables</p>
<table><tr><th>sku</th><th>quantity</th></tr><tr><td>A-1</td><td>2</td></tr></table>
<p>Conclusion</p>`

	segments, _, err := splitFileConversionContent(content)
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if got, want := len(segments), 3; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
	wants := []struct {
		content string
		tabular bool
	}{
		{content: "Introduction\n\nBetween tables\n\nConclusion", tabular: false},
		{content: "Source table header: name | amount", tabular: true},
		{content: "Source table header: sku | quantity", tabular: true},
	}
	for i, want := range wants {
		if segments[i].Tabular != want.tabular || !strings.Contains(segments[i].Content, want.content) {
			t.Fatalf("segment %d = %#v, want tabular=%v containing %q", i, segments[i], want.tabular, want.content)
		}
	}
}

func TestSplitFileConversionContentKeeps12001CharacterPlainTextInOneSegment(t *testing.T) {
	content := strings.Repeat("a", 12001)
	segments, tabular, err := splitFileConversionContent(content)
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if tabular {
		t.Fatal("splitFileConversionContent() tabular = true, want false")
	}
	if got, want := len(segments), 1; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
	if got := segments[0].Content; got != content {
		t.Fatalf("segment content length = %d, want %d", len(got), len(content))
	}
}

func TestSplitFileConversionContentKeepsMaxSizePlainTextInOneSegment(t *testing.T) {
	content := strings.Repeat("a", fileConversionMaxContentBytes)
	segments, tabular, err := splitFileConversionContent(content)
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if tabular {
		t.Fatal("splitFileConversionContent() tabular = true, want false")
	}
	if got, want := len(segments), 1; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
}

func TestSplitFileConversionContentRejectsContentOverMaxSize(t *testing.T) {
	content := strings.Repeat("a", fileConversionMaxContentBytes+1)
	segments, tabular, err := splitFileConversionContent(content)
	if tabular {
		t.Fatal("splitFileConversionContent() tabular = true, want false")
	}
	if err == nil || !strings.Contains(err.Error(), "content exceeds") {
		t.Fatalf("splitFileConversionContent() error = %v, want content limit error", err)
	}
	if len(segments) != 0 {
		t.Fatalf("len(segments) = %d, want 0", len(segments))
	}
}

func TestSplitFileConversionContentAllows400TableRows(t *testing.T) {
	content := markdownTableContent(400)
	segments, tabular, err := splitFileConversionContent(content)
	if err != nil {
		t.Fatalf("splitFileConversionContent() error = %v", err)
	}
	if !tabular {
		t.Fatal("splitFileConversionContent() tabular = false, want true")
	}
	if got, want := len(segments), fileConversionMaxTableSegments; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
}

func TestSplitFileConversionContentRejects401TableRowsForAllFormats(t *testing.T) {
	tests := map[string]string{
		"markdown": markdownTableContent(401),
		"html":     htmlTableContent(401),
		"csv":      csvTableContent(401),
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			segments, tabular, err := splitFileConversionContent(content)
			if !tabular {
				t.Fatal("splitFileConversionContent() tabular = false, want true")
			}
			if err == nil || !strings.Contains(err.Error(), "table segment limit") {
				t.Fatalf("splitFileConversionContent() error = %v, want table segment limit error", err)
			}
			if len(segments) != 0 {
				t.Fatalf("len(segments) = %d, want 0", len(segments))
			}
		})
	}
}

func markdownTableContent(rows int) string {
	var content strings.Builder
	content.WriteString("| name | amount |\n| --- | --- |\n")
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&content, "| customer-%d | %d |\n", i, i*10)
	}
	return content.String()
}

func htmlTableContent(rows int) string {
	var content strings.Builder
	content.WriteString("<table><tr><th>name</th><th>amount</th></tr>")
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&content, "<tr><td>customer-%d</td><td>%d</td></tr>", i, i*10)
	}
	content.WriteString("</table>")
	return content.String()
}

func csvTableContent(rows int) string {
	var content strings.Builder
	content.WriteString("name,amount\n")
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&content, "customer-%d,%d\n", i, i*10)
	}
	return content.String()
}

func TestValidateAndOrderSourceRowsRejectsMissingRow(t *testing.T) {
	one := 1
	parsed := fileConversionLLMResponse{
		Records: []fileConversionLLMRecord{{SourceRowIndex: &one}},
	}
	err := validateAndOrderSourceRows(&parsed, []int{1, 2})
	if err == nil || !strings.Contains(err.Error(), "returned 1 records for 2 source rows") {
		t.Fatalf("validateAndOrderSourceRows() error = %v, want row count mismatch", err)
	}
}

func TestValidateAndOrderSourceRowsOrdersRows(t *testing.T) {
	one, two := 1, 2
	parsed := fileConversionLLMResponse{
		Records: []fileConversionLLMRecord{
			{SourceRowIndex: &two},
			{SourceRowIndex: &one},
		},
	}
	if err := validateAndOrderSourceRows(&parsed, []int{1, 2}); err != nil {
		t.Fatalf("validateAndOrderSourceRows() returned error: %v", err)
	}
	if got := *parsed.Records[0].SourceRowIndex; got != 1 {
		t.Fatalf("first source row index = %d, want 1", got)
	}
}
