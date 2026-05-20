package logger_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var runtimeLoggingGuardRoots = []string{
	"internal",
	"middleware",
	"pkg",
	"routes",
}

var runtimeLoggingGuardExcludedDirs = []string{
	filepath.Join("internal", "migrations"),
	filepath.Join("internal", "migrationsv2"),
}

func TestRuntimeCodeDoesNotPrintLogsToStdout(t *testing.T) {
	root := findRepoRoot(t)
	var offenders []string

	for _, relRoot := range runtimeLoggingGuardRoots {
		walkRoot := filepath.Join(root, relRoot)
		if _, err := os.Stat(walkRoot); err != nil {
			continue
		}

		err := filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				relPath := mustRel(t, root, path)
				if shouldSkipRuntimeLoggingDir(relPath) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if strings.HasSuffix(path, "example.go") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for lineNo, line := range strings.Split(string(content), "\n") {
				trimmedLine := strings.TrimSpace(line)
				if strings.HasPrefix(trimmedLine, "//") {
					continue
				}
				if strings.Contains(line, "fmt.Print(") ||
					strings.Contains(line, "fmt.Printf(") ||
					strings.Contains(line, "fmt.Println(") {
					offenders = append(offenders, mustRel(t, root, path)+":"+strconv.Itoa(lineNo+1))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %q error = %v", walkRoot, err)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("runtime code must use pkg/logger instead of fmt.Print*: %s", strings.Join(offenders, ", "))
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}

func shouldSkipRuntimeLoggingDir(relPath string) bool {
	for _, excluded := range runtimeLoggingGuardExcludedDirs {
		if relPath == excluded || strings.HasPrefix(relPath, excluded+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func mustRel(t *testing.T, root, path string) string {
	t.Helper()

	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("filepath.Rel(%q, %q) error = %v", root, path, err)
	}
	return rel
}
