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

	segments, tabular := splitFileConversionContent(content.String())
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
	segments, tabular := splitFileConversionContent("name,amount\nAlice,10\nBob,20\n")
	if !tabular {
		t.Fatal("splitFileConversionContent() tabular = false, want true")
	}
	if got, want := segments[0].SourceRowIndexes, []int{1, 2}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("source row indexes = %v, want %v", got, want)
	}
}

func TestSplitFileConversionContentSplitsLongPlainText(t *testing.T) {
	content := strings.Repeat("a", fileConversionMaxTextRunes+1)
	segments, tabular := splitFileConversionContent(content)
	if tabular {
		t.Fatal("splitFileConversionContent() tabular = true, want false")
	}
	if got, want := len(segments), 2; got != want {
		t.Fatalf("len(segments) = %d, want %d", got, want)
	}
	if got, want := len([]rune(segments[0].Content)), fileConversionMaxTextRunes; got != want {
		t.Fatalf("first segment runes = %d, want %d", got, want)
	}
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
