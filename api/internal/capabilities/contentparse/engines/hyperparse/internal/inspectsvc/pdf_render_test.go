package inspectsvc

import "testing"

func TestResolveRenderPagesSequential(t *testing.T) {
	got, err := resolveRenderPages(4, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestResolveRenderPagesSparseSubset(t *testing.T) {
	got, err := resolveRenderPages(6, []int{5, 2, 2, 9})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 5}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestResolveRenderPagesOutOfRange(t *testing.T) {
	if _, err := resolveRenderPages(3, []int{7, 9}); err == nil {
		t.Fatal("expected error")
	}
}

func TestPageListIsSequential(t *testing.T) {
	if !pageListIsSequential([]int{1, 2, 3}, 3) {
		t.Fatal("expected sequential pages to be treated as full render")
	}
	if pageListIsSequential([]int{1, 3}, 3) {
		t.Fatal("unexpected sparse page list treated as sequential")
	}
}

func TestImagePathToDataURLMissing(t *testing.T) {
	if _, err := imagePathToDataURL("does-not-exist.png"); err == nil {
		t.Fatal("expected missing image error")
	}
}
