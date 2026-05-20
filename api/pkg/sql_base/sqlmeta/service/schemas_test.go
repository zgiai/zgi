package service

import "testing"

func TestIsProtectedSchemaName(t *testing.T) {
	cases := map[string]bool{
		"public":               true,
		" information_schema ": true,
		"pg_catalog":           true,
		"pg_toast":             true,
		"app_workspace":        false,
		"sqlmeta_test_123":     false,
	}

	for name, want := range cases {
		if got := isProtectedSchemaName(name); got != want {
			t.Fatalf("isProtectedSchemaName(%q) = %v, want %v", name, got, want)
		}
	}
}
