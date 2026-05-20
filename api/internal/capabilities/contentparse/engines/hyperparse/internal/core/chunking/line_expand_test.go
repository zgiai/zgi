package chunking

import (
	"strings"
	"testing"
)

func TestExpandTextsForMultilineSegments_UnsplitPreservesChunkID(t *testing.T) {
	in := []TextLike{{Order: 7, SourceTrace: "page#1", Text: "one line only", ChunkType: "paragraph"}}
	out := expandTextsForMultilineSegments(expandTextsForFormulaChunks(in))
	if len(out) != 1 || textLikeChunkID(out[0]) != "seg_7" {
		t.Fatalf("got %#v id=%q", out, textLikeChunkID(out[0]))
	}
}

func TestExpandTextsForMultilineSegments_SplitsBlocksByBlankLines(t *testing.T) {
	in := []TextLike{{Order: 5, SourceTrace: "page#1", Text: "使命\n\n第一行正文\n第二行正文", ChunkType: "paragraph"}}
	out := expandTextsForMultilineSegments(expandTextsForFormulaChunks(in))
	if len(out) != 2 {
		t.Fatalf("len=%d want 2: %#v", len(out), out)
	}
	if textLikeChunkID(out[0]) != "seg_5_L0" || textLikeChunkID(out[1]) != "seg_5_L1" {
		t.Fatalf("ids: %q %q", textLikeChunkID(out[0]), textLikeChunkID(out[1]))
	}
	if out[0].ChunkType != "heading" {
		t.Fatalf("short line 0 want heading, got %q", out[0].ChunkType)
	}
	if out[1].ChunkType != "" {
		t.Fatalf("multi-line paragraph block want empty ChunkType, got %q", out[1].ChunkType)
	}
	if out[1].Text != "第一行正文\n第二行正文" {
		t.Fatalf("block text=%q", out[1].Text)
	}
}

func TestExpandTextsForMultilineSegments_ListLineNotHeading(t *testing.T) {
	in := []TextLike{{Order: 1, SourceTrace: "page#1", Text: "章节\n\n- 列表项", ChunkType: "paragraph"}}
	out := expandTextsForMultilineSegments(in)
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].ChunkType != "heading" {
		t.Fatalf("line0=%q", out[0].ChunkType)
	}
	if out[1].ChunkType != "" {
		t.Fatalf("list line should not be pre-tagged heading, got %q", out[1].ChunkType)
	}
}

func TestExpandTextsForMultilineSegments_KVLineNotHeading(t *testing.T) {
	in := []TextLike{{Order: 1, SourceTrace: "page#1", Text: "标题\n\n键名: 值内容在这里", ChunkType: "paragraph"}}
	out := expandTextsForMultilineSegments(in)
	if len(out) != 2 || out[1].ChunkType != "" {
		t.Fatalf("second line ChunkType=%q", out[1].ChunkType)
	}
}

func TestBuild_SoftWrappedParagraphStaysSingleChunk(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       3,
				SourceTrace: "page#1",
				Text:        "这是一段正文第一行\n这是一段正文第二行",
				ChunkType:   "paragraph",
				GeomX:       10,
				GeomY:       100,
			},
		},
	}
	chunks := Build(input)
	var typesByID = map[string]string{}
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		typ, _ := c["type"].(string)
		if id != "" {
			typesByID[id] = typ
		}
	}
	if typesByID["seg_3"] != "paragraph" {
		t.Fatalf("seg_3 type=%v", typesByID)
	}
	if _, ok := typesByID["seg_3_L0"]; ok {
		t.Fatalf("should not split soft-wrapped paragraph: %v", typesByID)
	}
}

func TestBuild_BlankLineSplitEmitsHeadingAndParagraph(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       3,
				SourceTrace: "page#1",
				Text:        "小节标题\n\n" + strings.Repeat("文", 130),
				ChunkType:   "paragraph",
				GeomX:       10,
				GeomY:       100,
			},
		},
	}
	chunks := Build(input)
	var typesByID = map[string]string{}
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		typ, _ := c["type"].(string)
		if id != "" {
			typesByID[id] = typ
		}
	}
	if typesByID["seg_3_L0"] != "heading" {
		t.Fatalf("seg_3_L0 type=%v", typesByID)
	}
	if typesByID["seg_3_L1"] != "paragraph" {
		t.Fatalf("seg_3_L1 type=%v", typesByID)
	}
}

func TestBuild_HeadingThenParagraphWithoutBlankLine(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       11,
				SourceTrace: "page#1",
				Text:        "使命\n在 AGICTO，我们相信人工智能，尤其是大型语言模型，具有普惠价值。",
				ChunkType:   "paragraph",
				GeomX:       10,
				GeomY:       100,
			},
		},
	}
	chunks := Build(input)
	var typesByID = map[string]string{}
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		typ, _ := c["type"].(string)
		if id != "" {
			typesByID[id] = typ
		}
	}
	if typesByID["seg_11_L0"] != "heading" || typesByID["seg_11_L1"] != "paragraph" {
		t.Fatalf("unexpected split types: %v", typesByID)
	}
}

func TestSplitNativeTextBlocks_IgnoreShortGarbageLine(t *testing.T) {
	blocks := splitNativeTextBlocks("使命\nx\n在这里开始正文内容，长度足够。")
	if len(blocks) != 2 {
		t.Fatalf("blocks=%d want 2: %#v", len(blocks), blocks)
	}
	if blocks[0].chunkType != "heading" {
		t.Fatalf("block0 chunkType=%q", blocks[0].chunkType)
	}
	if blocks[1].text != "在这里开始正文内容，长度足够。" {
		t.Fatalf("block1 text=%q", blocks[1].text)
	}
}

func TestSplitNativeTextBlocks_HeadingAfterParagraphEndsWithPeriod(t *testing.T) {
	blocks := splitNativeTextBlocks("使命\n第一段正文结束.\n愿景\n第二段正文很长足够长。")
	if len(blocks) < 4 {
		t.Fatalf("blocks=%d want >=4: %#v", len(blocks), blocks)
	}
	if blocks[0].chunkType != "heading" || blocks[0].text != "使命" {
		t.Fatalf("block0=%+v", blocks[0])
	}
	if blocks[1].chunkType != "" || !strings.Contains(blocks[1].text, "第一段") {
		t.Fatalf("block1=%+v", blocks[1])
	}
	if blocks[2].chunkType != "heading" || blocks[2].text != "愿景" {
		t.Fatalf("block2=%+v", blocks[2])
	}
}

func TestSplitNativeTextBlocks_ShortHeadingShortFirstBodyLine(t *testing.T) {
	body1 := "在AGICTO我们相信人工智能尤其是大型语言模型。"
	blocks := splitNativeTextBlocks("使命\n" + body1)
	if len(blocks) != 2 || blocks[0].chunkType != "heading" || blocks[0].text != "使命" {
		t.Fatalf("want mission heading + body, got %#v", blocks)
	}
	if blocks[1].text != body1 {
		t.Fatalf("body=%q", blocks[1].text)
	}
}

func TestBuild_KVLineNotHeadingWhenColon(t *testing.T) {
	input := BuildInput{
		Texts: []TextLike{
			{Order: 9, SourceTrace: "page#1", Text: "名称: 值必须有实质内容", ChunkType: "paragraph"},
		},
	}
	chunks := Build(input)
	for _, c := range chunks {
		if c["chunk_id"] != "seg_9" {
			continue
		}
		if typ, _ := c["type"].(string); typ != "kv" {
			t.Fatalf("want kv, got %v", c)
		}
		return
	}
	t.Fatal("seg_9 not found")
}
