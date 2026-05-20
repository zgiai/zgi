package binutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestCandidatesIncludeCommonUnixInstallLocations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("common unix install locations are not used on windows")
	}
	want := filepath.Join("/opt/homebrew/bin", "pdftoppm")
	for _, candidate := range Candidates("pdftoppm") {
		if candidate == want {
			return
		}
	}
	t.Fatalf("expected candidates to include %q", want)
}

func TestResolveCommandEmpty(t *testing.T) {
	if _, err := ResolveCommand(""); err == nil {
		t.Fatal("expected empty command error")
	}
}
