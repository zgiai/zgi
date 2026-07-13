package indexing

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestParentChildTransformMergesShortLineParentChunksBeforeSplittingChildren(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Elements: []dto.ExtractElement{
			{Type: "text", Content: "alpha", Ordinal: 0},
			{Type: "text", Content: "beta", Ordinal: 1},
			{Type: "text", Content: "gamma", Ordinal: 2},
		},
	}
	options := &ProcessOptions{
		ProcessRule: map[string]interface{}{
			"parent_mode": "parent_child",
			"segmentation": map[string]interface{}{
				"separator":     "\n",
				"max_tokens":    50,
				"chunk_overlap": 0,
			},
			"subchunk_segmentation": map[string]interface{}{
				"separator":     "\n",
				"max_tokens":    50,
				"chunk_overlap": 0,
			},
		},
	}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Content != "alpha\nbeta\ngamma" {
		t.Fatalf("parent content = %q", got[0].Content)
	}
	if len(got[0].Children) != 3 {
		t.Fatalf("child count = %d, want 3", len(got[0].Children))
	}
	if got[0].Children[0].Content != "alpha" || got[0].Children[1].Content != "beta" || got[0].Children[2].Content != "gamma" {
		t.Fatalf("children = %#v", got[0].Children)
	}
	if got[0].Metadata["child_count"] != 3 {
		t.Fatalf("metadata child_count = %v, want 3", got[0].Metadata["child_count"])
	}
}

func TestParentChildElementGroupBuildsSizedParentGroups(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "text", Content: "aaaaaa", Ordinal: 0},
			{Type: "text", Content: "bbbbbb", Ordinal: 1},
			{Type: "text", Content: "cccccc", Ordinal: 2},
			{Type: "text", Content: "dddddd", Ordinal: 3},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":         "element_group",
		"parent_min_chars":    10,
		"parent_target_chars": 12,
		"parent_max_chars":    15,
		"child_min_chars":     3,
		"child_target_chars":  6,
		"child_max_chars":     8,
		"child_overlap_chars": 0,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Content != "aaaaaa\nbbbbbb" || got[1].Content != "cccccc\ndddddd" {
		t.Fatalf("contents = %#v", []string{got[0].Content, got[1].Content})
	}
	if got[0].Metadata["parent_mode"] != "element_group" || got[0].Metadata["source_char_count"] != 13 {
		t.Fatalf("metadata = %#v", got[0].Metadata)
	}
	if len(got[0].Children) == 0 || got[0].Children[0].Metadata["child_kind"] != "text" {
		t.Fatalf("children = %#v", got[0].Children)
	}
}

func TestParentChildElementGroupPrependsHierarchicalSectionPathToChildren(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "heading", Subtype: "h1", Content: "医院服务", Ordinal: 0},
			{Type: "text", Content: "服务总览", Ordinal: 1},
			{Type: "heading", Subtype: "h2", Content: "门诊服务", Ordinal: 2},
			{Type: "text", Content: "门诊说明", Ordinal: 3},
			{Type: "heading", Subtype: "h3", Content: "微信挂号", Ordinal: 4},
			{Type: "text", Content: "通过微信公众号完成挂号", Ordinal: 5},
			{Type: "heading", Subtype: "h2", Content: "住院服务", Ordinal: 6},
			{Type: "text", Content: "办理入院手续", Ordinal: 7},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":         "element_group",
		"parent_min_chars":    10,
		"parent_target_chars": 500,
		"parent_max_chars":    1000,
		"child_min_chars":     10,
		"child_target_chars":  300,
		"child_max_chars":     500,
		"child_overlap_chars": 0,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Children) != 4 {
		t.Fatalf("got = %#v, want one parent with four section children", got)
	}
	wantPaths := [][]string{
		{"医院服务"},
		{"医院服务", "门诊服务"},
		{"医院服务", "门诊服务", "微信挂号"},
		{"医院服务", "住院服务"},
	}
	for i, child := range got[0].Children {
		path, ok := child.Metadata[sectionPathMetadataKey].([]string)
		if !ok || !reflect.DeepEqual(path, wantPaths[i]) {
			t.Fatalf("child %d path = %#v, want %#v", i, child.Metadata[sectionPathMetadataKey], wantPaths[i])
		}
		prefix := strings.Join(wantPaths[i], " > ") + "\n\n"
		if !strings.HasPrefix(child.Content, prefix) {
			t.Fatalf("child %d content = %q, want prefix %q", i, child.Content, prefix)
		}
	}
	if !strings.Contains(got[0].Children[2].Content, "通过微信公众号完成挂号") {
		t.Fatalf("nested section content = %q", got[0].Children[2].Content)
	}
	if !strings.Contains(got[0].Children[3].Content, "办理入院手续") {
		t.Fatalf("sibling section content = %q", got[0].Children[3].Content)
	}
}

func TestParentChildElementGroupPreservesSectionPathAcrossParentGroups(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	body := strings.Repeat("正文", 10)
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "heading", Subtype: "h1", Content: "根目录", Ordinal: 0},
			{Type: "text", Content: body, Ordinal: 1},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":         "element_group",
		"parent_min_chars":    1,
		"parent_target_chars": 4,
		"parent_max_chars":    5,
		"child_min_chars":     5,
		"child_target_chars":  80,
		"child_max_chars":     100,
		"child_overlap_chars": 0,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 2 || len(got[1].Children) != 1 {
		t.Fatalf("got = %#v, want body in a separate parent group", got)
	}
	child := got[1].Children[0]
	if !strings.HasPrefix(child.Content, "根目录\n\n") || !strings.Contains(child.Content, body) {
		t.Fatalf("child content = %q, want inherited root path", child.Content)
	}
	if path, _ := child.Metadata[sectionPathMetadataKey].([]string); !reflect.DeepEqual(path, []string{"根目录"}) {
		t.Fatalf("section path = %#v", child.Metadata[sectionPathMetadataKey])
	}
}

func TestAnnotateElementSectionPathsUsesProviderHeadingMetadata(t *testing.T) {
	elements := []dto.ExtractElement{
		{Type: "heading", Subtype: "title", Content: "文档标题", Ordinal: 0},
		{Type: "heading", Subtype: "section_header", Content: "一级栏目", Ordinal: 1},
		{Type: "heading", Content: "详细栏目", Ordinal: 2, Metadata: map[string]any{
			"payload": map[string]any{"mineru_text_level": float64(3)},
		}},
		{Type: "text", Content: "正文", Ordinal: 3},
	}

	annotated := annotateElementSectionPaths(elements)
	path, _ := annotated[3].Metadata[sectionPathMetadataKey].([]string)
	want := []string{"文档标题", "一级栏目", "详细栏目"}
	if !reflect.DeepEqual(path, want) {
		t.Fatalf("path = %#v, want %#v", path, want)
	}
	if len(elements[3].Metadata) != 0 {
		t.Fatalf("source elements were mutated: %#v", elements[3].Metadata)
	}
}

func TestElementSectionPathIncludesTitleThroughH4AndExcludesDeeperHeadings(t *testing.T) {
	elements := []dto.ExtractElement{
		{Type: "heading", Subtype: "title", Content: "文档标题", Ordinal: 0},
		{Type: "heading", Subtype: "h1", Content: "一级", Ordinal: 1},
		{Type: "heading", Subtype: "h2", Content: "二级", Ordinal: 2},
		{Type: "heading", Subtype: "h3", Content: "三级", Ordinal: 3},
		{Type: "heading", Subtype: "h4", Content: "四级", Ordinal: 4},
		{Type: "heading", Subtype: "h5", Content: "五级甲", Ordinal: 5},
		{Type: "text", Content: "甲正文", Ordinal: 6},
		{Type: "heading", Subtype: "h5", Content: "五级乙", Ordinal: 7},
		{Type: "text", Content: "乙正文", Ordinal: 8},
	}

	annotated := annotateElementSectionPaths(elements)
	wantVisible := []string{"文档标题", "一级", "二级", "三级", "四级"}
	for _, index := range []int{6, 8} {
		path, _ := annotated[index].Metadata[sectionPathMetadataKey].([]string)
		if !reflect.DeepEqual(path, wantVisible) {
			t.Fatalf("element %d visible path = %#v, want %#v", index, path, wantVisible)
		}
	}
	firstScope, _ := annotated[6].Metadata[sectionScopeMetadataKey].([]string)
	secondScope, _ := annotated[8].Metadata[sectionScopeMetadataKey].([]string)
	if reflect.DeepEqual(firstScope, secondScope) || firstScope[len(firstScope)-1] != "五级甲" || secondScope[len(secondScope)-1] != "五级乙" {
		t.Fatalf("full scopes = %#v / %#v, want distinct H5 boundaries", firstScope, secondScope)
	}

	processor := &ParentChildIndexProcessor{}
	children := processor.buildElementGroupChildren(annotated[5:], elementGroupParams{
		ChildMinChars:     5,
		ChildTargetChars:  100,
		ChildMaxChars:     200,
		ChildOverlapChars: 0,
		TableMaxChars:     200,
	})
	if len(children) != 2 {
		t.Fatalf("children = %#v, want separate children for the two H5 scopes", children)
	}
	wantPrefix := "文档标题 > 一级 > 二级 > 三级 > 四级\n\n"
	for i, child := range children {
		if !strings.HasPrefix(child.Content, wantPrefix) || strings.Contains(child.Content, "章节路径：") {
			t.Fatalf("child %d content = %q", i, child.Content)
		}
		if directoryLine := strings.SplitN(child.Content, "\n", 2)[0]; directoryLine != strings.Join(wantVisible, " > ") {
			t.Fatalf("child %d directory = %q, want title through H4", i, directoryLine)
		}
		if _, exists := child.Metadata[sectionScopeMetadataKey]; exists {
			t.Fatalf("child %d leaked internal section scope: %#v", i, child.Metadata)
		}
	}
}

func TestParentChildElementGroupPrependsSectionPathToAtomicAndTableChildren(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "heading", Subtype: "h1", Content: "检查指南", Ordinal: 0},
			{Type: "image", Content: "检查流程图", Ordinal: 1},
			{Type: "table", Content: "| 项目 | 地点 |\n| --- | --- |\n| CT | 一楼 |", Ordinal: 2},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":           "element_group",
		"parent_min_chars":      10,
		"parent_target_chars":   300,
		"parent_max_chars":      500,
		"child_min_chars":       5,
		"child_target_chars":    80,
		"child_max_chars":       120,
		"child_overlap_chars":   0,
		"table_child_max_chars": 120,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Children) != 3 {
		t.Fatalf("got = %#v, want heading, image, and table children", got)
	}
	for i, child := range got[0].Children {
		if !strings.HasPrefix(child.Content, "检查指南\n\n") {
			t.Fatalf("child %d content = %q", i, child.Content)
		}
	}
	if got[0].Children[1].Metadata["child_kind"] != "image" || got[0].Children[2].Metadata["child_kind"] != "table" {
		t.Fatalf("children = %#v", got[0].Children)
	}
}

func TestParentChildElementGroupSectionPrefixCountsTowardChildLimit(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "heading", Subtype: "h1", Content: "门诊服务", Ordinal: 0},
			{Type: "text", Content: strings.Repeat("挂号缴费检查。", 20), Ordinal: 1},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":         "element_group",
		"parent_min_chars":    10,
		"parent_target_chars": 500,
		"parent_max_chars":    1000,
		"child_min_chars":     20,
		"child_target_chars":  50,
		"child_max_chars":     60,
		"child_overlap_chars": 5,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Children) < 2 {
		t.Fatalf("got = %#v, want multiple child chunks", got)
	}
	for i, child := range got[0].Children {
		if runes := len([]rune(child.Content)); runes > 60 {
			t.Fatalf("child %d length = %d, want <= 60: %q", i, runes, child.Content)
		}
	}
}

func TestParentChildElementGroupFallsBackToParagraphForUnstructuredInput(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Elements: []dto.ExtractElement{
			{Type: "text", Content: "plain text input without structural parser metadata", Ordinal: 0},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode": "element_group",
		"segmentation": map[string]interface{}{
			"separator":     "\n",
			"max_tokens":    500,
			"chunk_overlap": 0,
		},
		"subchunk_segmentation": map[string]interface{}{
			"separator":     "\n",
			"max_tokens":    100,
			"chunk_overlap": 0,
		},
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	metadata := got[0].Metadata
	if metadata["requested_parent_mode"] != "element_group" || metadata["effective_parent_mode"] != "paragraph" {
		t.Fatalf("fallback metadata = %#v", metadata)
	}
	if metadata["fallback_reason"] != "structured_elements_unavailable" {
		t.Fatalf("fallback reason = %#v", metadata["fallback_reason"])
	}
}

func TestParentChildElementGroupFallsBackToParagraphForMarkdownOnlyInput(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{Markdown: "markdown-only content"}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode": "element_group",
		"segmentation": map[string]interface{}{
			"separator":     "\n",
			"max_tokens":    500,
			"chunk_overlap": 0,
		},
		"subchunk_segmentation": map[string]interface{}{
			"separator":     "\n",
			"max_tokens":    100,
			"chunk_overlap": 0,
		},
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 || got[0].Content != "markdown-only content" {
		t.Fatalf("got = %#v, want markdown paragraph fallback", got)
	}
	if got[0].Metadata["effective_parent_mode"] != "paragraph" {
		t.Fatalf("fallback metadata = %#v", got[0].Metadata)
	}
}

func TestParentChildElementGroupKeepsImageAndFormulaAtomic(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "text", Content: "intro text", Ordinal: 0},
			{Type: "image", Page: 2, Ordinal: 1, Metadata: map[string]any{"caption": "system diagram"}},
			{Type: "formula", Page: 3, Ordinal: 2, Metadata: map[string]any{"latex": "E=mc^2"}},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":         "element_group",
		"parent_min_chars":    10,
		"parent_target_chars": 80,
		"parent_max_chars":    120,
		"child_min_chars":     5,
		"child_target_chars":  20,
		"child_max_chars":     30,
		"child_overlap_chars": 0,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if len(got[0].Children) != 3 {
		t.Fatalf("child count = %d, want 3: %#v", len(got[0].Children), got[0].Children)
	}
	if got[0].Children[1].Content != "system diagram" || got[0].Children[1].Metadata["child_kind"] != "image" {
		t.Fatalf("image child = %#v", got[0].Children[1])
	}
	if got[0].Children[2].Content != "E=mc^2" || got[0].Children[2].Metadata["child_kind"] != "formula" {
		t.Fatalf("formula child = %#v", got[0].Children[2])
	}
}

func TestParentChildElementGroupSplitsLongTables(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	table := "| Name | Value |\n| --- | --- |\n" +
		"| Alpha | 11111111111111111111 |\n" +
		"| Beta | 22222222222222222222 |\n" +
		"| Gamma | 33333333333333333333 |"
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "table", Content: table, Ordinal: 0},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":           "element_group",
		"parent_min_chars":      10,
		"parent_target_chars":   200,
		"parent_max_chars":      300,
		"child_min_chars":       10,
		"child_target_chars":    40,
		"child_max_chars":       90,
		"child_overlap_chars":   0,
		"table_child_max_chars": 90,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if len(got[0].Children) < 2 {
		t.Fatalf("table children = %d, want at least 2: %#v", len(got[0].Children), got[0].Children)
	}
	for i, child := range got[0].Children {
		if child.Metadata["child_kind"] != "table" {
			t.Fatalf("child %d kind = %#v", i, child.Metadata["child_kind"])
		}
		if !strings.Contains(child.Content, "| Name | Value |") {
			t.Fatalf("child %d missing repeated header: %q", i, child.Content)
		}
	}
}

func TestParentChildElementGroupSplitsSingleOversizedTableRow(t *testing.T) {
	processor := &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(nil, nil, nil, ""),
	}
	table := "| Name | Notes |\n| --- | --- |\n| Alpha | " + strings.Repeat("long-value-", 20) + " |"
	output := &dto.ExtractOutput{
		Metadata: map[string]any{"structured_elements": true},
		Elements: []dto.ExtractElement{
			{Type: "table", Content: table, Ordinal: 0},
		},
	}
	options := &ProcessOptions{ProcessRule: map[string]interface{}{
		"parent_mode":           "element_group",
		"parent_min_chars":      10,
		"parent_target_chars":   400,
		"parent_max_chars":      500,
		"child_min_chars":       10,
		"child_target_chars":    40,
		"child_max_chars":       60,
		"child_overlap_chars":   0,
		"table_child_max_chars": 60,
	}}

	got, err := processor.Transform(context.Background(), output, options)
	if err != nil {
		t.Fatalf("Transform returned error: %v", err)
	}
	if len(got) != 1 || len(got[0].Children) < 2 {
		t.Fatalf("got = %#v, want one parent with split row children", got)
	}
	for i, child := range got[0].Children {
		if len([]rune(child.Content)) > 60 {
			t.Fatalf("child %d length = %d, want <= 60: %q", i, len([]rune(child.Content)), child.Content)
		}
		if child.Metadata["table_row_split_count"] == nil {
			t.Fatalf("child %d missing row split metadata: %#v", i, child.Metadata)
		}
	}
}

func TestMergeSmallParentChunksCombinesAdjacentShortChunks(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{
			Content: "line one",
			Metadata: map[string]any{
				"doc_id":       "old-parent-id",
				"doc_hash":     "old-parent-hash",
				"chunk_index":  0,
				"total_chunks": 3,
				"element_type": "text",
			},
		},
		{
			Content: "line two",
			Metadata: map[string]any{
				"element_type": "text",
			},
		},
		{
			Content: "line three",
			Metadata: map[string]any{
				"element_type": "text",
			},
		},
	}

	got := mergeSmallParentChunks(chunks, 32)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Content != "line one\nline two\nline three" {
		t.Fatalf("content = %q", got[0].Content)
	}
	if _, ok := got[0].Metadata["doc_id"]; ok {
		t.Fatal("merged metadata should drop doc_id so addChunkMetadata can regenerate it")
	}
	if _, ok := got[0].Metadata["doc_hash"]; ok {
		t.Fatal("merged metadata should drop doc_hash so addChunkMetadata can regenerate it")
	}
	if _, ok := got[0].Metadata["chunk_index"]; ok {
		t.Fatal("merged metadata should drop stale chunk_index")
	}
}

func TestMergeSmallParentChunksFlushesNearTarget(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{Content: "12345", Metadata: map[string]any{"element_type": "text"}},
		{Content: "67890", Metadata: map[string]any{"element_type": "text"}},
		{Content: "abcde", Metadata: map[string]any{"element_type": "text"}},
	}

	got := mergeSmallParentChunks(chunks, 11)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Content != "12345\n67890" {
		t.Fatalf("first content = %q", got[0].Content)
	}
	if got[1].Content != "abcde" {
		t.Fatalf("second content = %q", got[1].Content)
	}
}

func TestMergeSmallParentChunksKeepsOversizedChunkIntact(t *testing.T) {
	longContent := strings.Repeat("x", 20)
	chunks := []dto.TransformedChunk{
		{Content: longContent, Metadata: map[string]any{"element_type": "text"}},
		{Content: "short", Metadata: map[string]any{"element_type": "text"}},
		{Content: "tail", Metadata: map[string]any{"element_type": "text"}},
	}

	got := mergeSmallParentChunks(chunks, 10)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Content != longContent {
		t.Fatalf("first content length = %d, want %d", len([]rune(got[0].Content)), len([]rune(longContent)))
	}
	if got[1].Content != "short\ntail" {
		t.Fatalf("second content = %q", got[1].Content)
	}
}

func TestMergeSmallParentChunksDoesNotMergeStandaloneChunks(t *testing.T) {
	chunks := []dto.TransformedChunk{
		{Content: "before", Metadata: map[string]any{"element_type": "text"}},
		{Content: "| a | b |", Metadata: map[string]any{"element_type": "table"}},
		{Content: "after", Metadata: map[string]any{"element_type": "text"}},
		{Content: "tail", Metadata: map[string]any{"element_type": "text"}},
	}

	got := mergeSmallParentChunks(chunks, 100)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].Content != "before" {
		t.Fatalf("first content = %q", got[0].Content)
	}
	if got[1].Content != "| a | b |" {
		t.Fatalf("standalone content = %q", got[1].Content)
	}
	if got[2].Content != "after\ntail" {
		t.Fatalf("tail content = %q", got[2].Content)
	}
}

func TestParentChunkMergeTarget(t *testing.T) {
	if got := parentChunkMergeTarget(&SegmentationRule{MaxTokens: 128}); got != 128 {
		t.Fatalf("target = %d, want 128", got)
	}
	if got := parentChunkMergeTarget(nil); got != defaultParentChunkMergeTarget {
		t.Fatalf("default target = %d, want %d", got, defaultParentChunkMergeTarget)
	}
}

func TestBuildSubchunkSeparatorsPreservesNewlineSeparator(t *testing.T) {
	fixedSeparator, separators := buildSubchunkSeparators("\n")

	if fixedSeparator != "\n" {
		t.Fatalf("fixedSeparator = %q, want newline", fixedSeparator)
	}
	if len(separators) == 0 || separators[0] != "\n" {
		t.Fatalf("first separator = %q, want newline", separators[0])
	}
}
