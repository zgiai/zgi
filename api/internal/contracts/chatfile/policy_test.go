package chatfile

import (
	"errors"
	"testing"
)

func TestAdmissionBoundaries(t *testing.T) {
	p := Resolve(nil, 50, 10, 5)
	for _, tt := range []struct {
		ext  string
		size int64
		want error
	}{
		{"pdf", 50 * MiB, nil}, {"pdf", 50*MiB + 1, ErrFileTooLarge},
		{"PNG", 10 * MiB, nil}, {"png", 10*MiB + 1, ErrFileTooLarge},
		{"zip", 1, ErrUnsupportedType}, {"mp4", 1, ErrUnsupportedType}, {"py", 1, ErrUnsupportedType},
		{"json", 1, nil}, {"pdf", -1, ErrFileTooLarge},
	} {
		if err := p.ValidateFile(tt.ext, tt.size); !errors.Is(err, tt.want) {
			t.Errorf("ValidateFile(%q, %d)=%v, want %v", tt.ext, tt.size, err, tt.want)
		}
	}
	if err := p.ValidateTurn([]int64{50 * MiB, 50 * MiB}); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateTurn([]int64{50 * MiB, 50 * MiB, 1}); !errors.Is(err, ErrTurnTooLarge) {
		t.Fatalf("overflow=%v", err)
	}
	if err := p.ValidateTurn([]int64{1, 1, 1, 1, 1, 1}); !errors.Is(err, ErrTooManyFiles) {
		t.Fatalf("count=%v", err)
	}
}

func TestPolicyTypesAndOwnership(t *testing.T) {
	for _, kind := range []string{"custom", "audio", "video", "*", "Document"} {
		if err := (&Policy{AllowedFileTypes: []string{kind}}).Validate(); err == nil {
			t.Errorf("accepted %q", kind)
		}
	}
	p := &Policy{AllowedFileTypes: []string{"document"}}
	copy := Clone(p)
	copy.AllowedFileTypes[0] = "image"
	if p.AllowedFileTypes[0] != "document" {
		t.Fatal("clone aliases original")
	}
	effective := Resolve(p, 100, 10, 5)
	if err := effective.ValidateFile("png", 1); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("image=%v", err)
	}
	if err := effective.ValidateFile("pdf", 70*MiB); err != nil {
		t.Fatalf("old 64MiB boundary: %v", err)
	}
	if err := Resolve(&Policy{}, 50, 10, 5).ValidateFile("pdf", 1); err == nil {
		t.Fatal("empty policy broadened")
	}
}
