package pdf

import "testing"

func TestFindNextContentPDFOpSequence(t *testing.T) {
	block := []byte("BT /F1 12 Tf 1 0 0 1 100 700 Tm (Hello) Tj ET")
	cursor := 0
	want := []string{"bt", "tf", "tm", "tjl", "et"}
	for i, expected := range want {
		kind, _, end := findNextContentPDFOp(block, cursor)
		if kind != expected {
			t.Fatalf("step %d kind=%q want=%q", i, kind, expected)
		}
		cursor = end
	}
}

func TestFindNextContentPDFOpTJArray(t *testing.T) {
	block := []byte("[ (Doc) 120 (Still) ] TJ")
	kind, _, _ := findNextContentPDFOp(block, 0)
	if kind != "tjarr" {
		t.Fatalf("kind=%q want=tjarr", kind)
	}
}
