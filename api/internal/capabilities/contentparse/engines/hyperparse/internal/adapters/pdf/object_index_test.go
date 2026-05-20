package pdf

import (
	"reflect"
	"testing"
)

func TestBuildPDFObjectIndexSinglePassHandlesCRLineEndings(t *testing.T) {
	data := []byte("1 0 obj\r<< /Type /Catalog >>\rendobj\r2 0 obj\r<< /Type /Page >>\rendobj\r")

	unregister := RegisterObjectIndexForParse(data)
	defer unregister()

	got := IndexedDirectObjectNumbers(data)
	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("indexed objects=%v want=%v", got, want)
	}
	if _, err := ExtractObjectBlockByNumberBytes(data, 2, ValidationModeRelaxed); err != nil {
		t.Fatalf("object 2 should resolve with index: %v", err)
	}
}
