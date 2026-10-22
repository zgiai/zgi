package chatfile

import (
	"encoding/json"
	"errors"
	"slices"
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
	for _, kind := range []string{"custom", "*", "Document"} {
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

func TestExtendedPolicyKindsCustomAndCount(t *testing.T) {
	media := &Policy{AllowedFileTypes: []string{"audio", "video"}, NumberLimits: 8}
	if err := media.Validate(); err != nil {
		t.Fatal(err)
	}
	effective := ResolveLimits(media, Limits{DocumentMiB: 50, ImageMiB: 10, AudioMiB: 20, VideoMiB: 80})
	if effective.NumberLimit != 8 || effective.ValidateFile("mp3", 20*MiB) != nil || !errors.Is(effective.ValidateFile("mp3", 20*MiB+1), ErrFileTooLarge) ||
		effective.ValidateFile("mp4", 80*MiB) != nil || !errors.Is(effective.ValidateFile("pdf", 1), ErrUnsupportedType) {
		t.Fatalf("media policy = %#v", effective)
	}
	custom := &Policy{AllowedFileTypes: []string{"custom"}, AllowedFileExtensions: []string{".ZIP", "dwg", "zip"}}
	custom.Normalize()
	if err := custom.Validate(); err != nil || !slices.Equal(custom.AllowedFileExtensions, []string{"zip", "dwg"}) {
		t.Fatalf("custom = %#v, %v", custom, err)
	}
	if Resolve(custom, 50, 10, 5).ValidateFile("dwg", 1) != nil {
		t.Fatal("custom extension rejected")
	}
	for name, invalid := range map[string]*Policy{
		"custom with others":    {AllowedFileTypes: []string{"custom", "image"}, AllowedFileExtensions: []string{"zip"}},
		"custom without list":   {AllowedFileTypes: []string{"custom"}},
		"executable":            {AllowedFileTypes: []string{"custom"}, AllowedFileExtensions: []string{"exe"}},
		"extensions without it": {AllowedFileTypes: []string{"document"}, AllowedFileExtensions: []string{"zip"}},
		"too many files":        {AllowedFileTypes: []string{"document"}, NumberLimits: 11},
	} {
		if invalid.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// Policies saved before the new fields keep their JSON.
	data, _ := json.Marshal(&Policy{AllowedFileTypes: []string{"document"}})
	if string(data) != `{"allowed_file_types":["document"]}` {
		t.Fatalf("legacy JSON = %s", data)
	}
	if !Parsed("pdf") || Parsed("mp3") || Parsed("png") {
		t.Fatal("parsed classification")
	}
}
