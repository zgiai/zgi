package hyperparse

import "testing"

func TestFormatFromFilename(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"a.pdf", "pdf"},
		{"x.DOCX", "docx"},
		{"readme.md", "markdown"},
		{"p.PNG", "image"},
		{"noext", ""},
	}
	for _, tt := range tests {
		if g := FormatFromFilename(tt.name); g != tt.want {
			t.Errorf("FormatFromFilename(%q)=%q want %q", tt.name, g, tt.want)
		}
	}
}
