package pdf

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// findModuleRoot walks up from wd until go.mod exists.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func TestFullDocumentTestdocPerf(t *testing.T) {
	if os.Getenv("DOCSTILL_RUN_PERF") == "" {
		t.Skip("set DOCSTILL_RUN_PERF=1 to run testdoc PDF benchmarks (slow)")
	}
	var dir string
	if e := os.Getenv("DOCSTILL_TESTDOC"); e != "" {
		dir = e
	} else {
		root := findModuleRoot(t)
		if root == "" {
			t.Skip("cannot find go.mod; run tests from module root or set DOCSTILL_TESTDOC")
		}
		dir = filepath.Join(root, "testdoc")
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Skipf("no PDFs in %s (place 作业单.pdf etc. here or set DOCSTILL_TESTDOC)", dir)
	}
	sort.Strings(matches)

	mode := "relaxed"
	if m := os.Getenv("DOCSTILL_VALIDATE_MODE"); m != "" {
		mode = m
	}

	for _, path := range matches {
		path := path
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			_, err := ParseFullDocument(path, mode)
			d := time.Since(start)
			if err != nil {
				t.Fatalf("ParseFullDocument: %v", err)
			}
			t.Logf("%s: %s", name, d.Round(time.Millisecond))
		})
	}
}

func TestFullDocumentSinglePDF(t *testing.T) {
	if os.Getenv("DOCSTILL_RUN_PERF") == "" {
		t.Skip("set DOCSTILL_RUN_PERF=1 and DOCSTILL_PERF_PDF to a pdf path (slow)")
	}
	p := os.Getenv("DOCSTILL_PERF_PDF")
	if strings.TrimSpace(p) == "" {
		t.Skip("set DOCSTILL_PERF_PDF to a pdf path under testdoc, e.g. testdoc/作业单.pdf")
	}
	if !filepath.IsAbs(p) {
		root := findModuleRoot(t)
		if root == "" {
			t.Fatal("cannot find go.mod")
		}
		p = filepath.Join(root, p)
	}
	mode := "relaxed"
	if m := os.Getenv("DOCSTILL_VALIDATE_MODE"); m != "" {
		mode = m
	}
	start := time.Now()
	_, err := ParseFullDocument(p, mode)
	d := time.Since(start)
	if err != nil {
		t.Fatalf("ParseFullDocument: %v", err)
	}
	t.Logf("%s: %s", p, d.Round(time.Millisecond))
}
