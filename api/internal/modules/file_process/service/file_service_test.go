package service

import "testing"

func TestIsAssetProcessableExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{ext: "pdf", want: true},
		{ext: ".docx", want: true},
		{ext: "png", want: true},
		{ext: ".jpg", want: true},
		{ext: "webp", want: true},
		{ext: "mp4", want: false},
		{ext: "mp3", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := isAssetProcessableExtension(tt.ext); got != tt.want {
				t.Fatalf("processable=%v want %v", got, tt.want)
			}
		})
	}
}
