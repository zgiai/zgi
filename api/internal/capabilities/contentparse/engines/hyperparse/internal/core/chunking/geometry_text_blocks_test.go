package chunking

import "testing"

func TestTextLikesFromGeometryLines_BreaksOnColumnShiftAndSectionBoundary(t *testing.T) {
	input := []GeometryLineLike{
		{Order: 11, PageIndex: 1, SourceTrace: "page#1", Text: "Invoice number 1410483071", GeomX: 75, GeomY: 72},
		{Order: 12, PageIndex: 1, SourceTrace: "page#1", Text: "Your electricity bill at a glance", GeomX: 4, GeomY: 69},
		{Order: 13, PageIndex: 1, SourceTrace: "page#1", Text: "Full details of your account are on the back of this bill", GeomX: 4, GeomY: 67},
		{Order: 14, PageIndex: 1, SourceTrace: "page#1", Text: "Billing period", GeomX: 6, GeomY: 63},
	}

	got := TextLikesFromGeometryLines(input, map[int]PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	})

	if len(got) != 3 {
		t.Fatalf("len=%d want 3: %#v", len(got), got)
	}
	if got[0].Text != "Invoice number 1410483071" {
		t.Fatalf("block0=%q", got[0].Text)
	}
	if got[1].Text != "Your electricity bill at a glance\nFull details of your account are on the back of this bill" {
		t.Fatalf("block1=%q", got[1].Text)
	}
	if got[2].Text != "Billing period" {
		t.Fatalf("block2=%q", got[2].Text)
	}
}

func TestTextLikesFromGeometryLines_KeepsPlainMultilineParagraphTogether(t *testing.T) {
	input := []GeometryLineLike{
		{Order: 37, PageIndex: 1, SourceTrace: "page#1", Text: "Payment terms are 14 days from date of bill issue or", GeomX: 4, GeomY: 28},
		{Order: 38, PageIndex: 1, SourceTrace: "page#1", Text: "immediately if overdue.", GeomX: 4, GeomY: 27},
		{Order: 39, PageIndex: 1, SourceTrace: "page#1", Text: "Information on the Fuel Mix and environmental impact is on", GeomX: 4, GeomY: 26},
		{Order: 40, PageIndex: 1, SourceTrace: "page#1", Text: "the back of this bill.", GeomX: 4, GeomY: 25},
	}

	got := TextLikesFromGeometryLines(input, map[int]PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	})

	if len(got) != 1 {
		t.Fatalf("len=%d want 1: %#v", len(got), got)
	}
	if got[0].Text != "Payment terms are 14 days from date of bill issue or\nimmediately if overdue.\nInformation on the Fuel Mix and environmental impact is on\nthe back of this bill." {
		t.Fatalf("text=%q", got[0].Text)
	}
}

func TestShouldPreferGeometryTextBlocks_WhenNativeSegmentSpansBarcodeGap(t *testing.T) {
	texts := []TextLike{
		{
			Order:       29,
			SourceTrace: "page#1 obj#4",
			Text:        "Payment terms are 14 days from date of bill issue or\nimmediately if overdue.\nInformation on the Fuel Mix and environmental impact is on\nthe back of this bill.\n0001 9518578800 000000182010 798258",
			ChunkType:   "paragraph",
			GeomX:       4,
			GeomY:       28,
		},
	}
	lines := []GeometryLineLike{
		{Order: 37, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "Payment terms are 14 days from date of bill issue or", GeomX: 4, GeomY: 28},
		{Order: 38, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "immediately if overdue.", GeomX: 4, GeomY: 27},
		{Order: 39, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "Information on the Fuel Mix and environmental impact is on", GeomX: 4, GeomY: 26},
		{Order: 40, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "the back of this bill.", GeomX: 4, GeomY: 25},
		{Order: 41, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "0001 9518578800 000000182010 798258", GeomX: 10, GeomY: 14},
	}

	got := ShouldPreferGeometryTextBlocks(texts, lines, map[int]PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	})

	if !got {
		t.Fatalf("expected geometry text blocks to be preferred")
	}
}

func TestShouldPreferGeometryTextBlocks_KeepsTightParagraphOnNativeSegments(t *testing.T) {
	texts := []TextLike{
		{
			Order:       28,
			SourceTrace: "page#1 obj#4",
			Text:        "Payment terms are 14 days from date of bill issue or\nimmediately if overdue.\nInformation on the Fuel Mix and environmental impact is on\nthe back of this bill.",
			ChunkType:   "paragraph",
			GeomX:       4,
			GeomY:       28,
		},
	}
	lines := []GeometryLineLike{
		{Order: 37, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "Payment terms are 14 days from date of bill issue or", GeomX: 4, GeomY: 28},
		{Order: 38, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "immediately if overdue.", GeomX: 4, GeomY: 27},
		{Order: 39, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "Information on the Fuel Mix and environmental impact is on", GeomX: 4, GeomY: 26},
		{Order: 40, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "the back of this bill.", GeomX: 4, GeomY: 25},
	}

	got := ShouldPreferGeometryTextBlocks(texts, lines, map[int]PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	})

	if got {
		t.Fatalf("expected native segments to remain preferred for tight paragraph")
	}
}
