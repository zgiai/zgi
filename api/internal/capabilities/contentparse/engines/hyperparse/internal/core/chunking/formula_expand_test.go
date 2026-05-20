package chunking

import (
	"strings"
	"testing"
)

func TestExpandTextsForFormulaChunks_DollarInline(t *testing.T) {
	in := []TextLike{
		{Order: 10, SourceTrace: "page#1", Text: "因此 $E=mc^2$ 成立", ChunkType: "paragraph"},
	}
	out := expandTextsForFormulaChunks(in)
	if len(out) != 3 {
		t.Fatalf("len=%d want 3: %#v", len(out), out)
	}
	if textLikeChunkID(out[0]) != "seg_10_m0" || textLikeChunkID(out[1]) != "seg_10_m1" || textLikeChunkID(out[2]) != "seg_10_m2" {
		t.Fatalf("chunk ids: %q %q %q", textLikeChunkID(out[0]), textLikeChunkID(out[1]), textLikeChunkID(out[2]))
	}
	if out[0].Order != 0 || out[1].Order != 1 || out[2].Order != 2 {
		t.Fatalf("orders: %d %d %d", out[0].Order, out[1].Order, out[2].Order)
	}
}

func TestExpandTextsForFormulaChunks_UnsplitPreservesSegChunkID(t *testing.T) {
	in := []TextLike{{Order: 7, SourceTrace: "page#1", Text: "x", ChunkType: "paragraph"}}
	out := expandTextsForFormulaChunks(in)
	if len(out) != 1 || textLikeChunkID(out[0]) != "seg_7" {
		t.Fatalf("got %#v id=%q", out, textLikeChunkID(out[0]))
	}
}

func TestBuild_SplitParagraphEmitsSeparateFormulaChunk(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       2,
				SourceTrace: "page#1",
				Text:        "说明如下。\n$ST_i(总) = T_i(统筹) + T_i(修龄)$\n以上为定义。",
				ChunkType:   "paragraph",
				GeomX:       10,
				GeomY:       100,
			},
		},
	}
	chunks := Build(input)
	var types []string
	var ids []string
	for _, c := range chunks {
		typ, _ := c["type"].(string)
		types = append(types, typ)
		ids = append(ids, c["chunk_id"].(string))
	}
	if len(types) < 2 {
		t.Fatalf("want >=2 chunks, got types=%v ids=%v", types, ids)
	}
	var hasFormula bool
	for _, c := range chunks {
		if typ, _ := c["type"].(string); typ == "formula" {
			hasFormula = true
			txt, _ := c["text"].(string)
			if !strings.Contains(txt, "formula:") {
				t.Fatalf("formula text: %q", txt)
			}
		}
	}
	if !hasFormula {
		t.Fatalf("no formula chunk: types=%v", types)
	}
}
