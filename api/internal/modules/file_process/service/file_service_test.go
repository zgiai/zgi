package service

import (
	"testing"

	"github.com/zgiai/zgi/api/config"
)

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

func TestGetUploadConfigIncludesQueueAndConcurrencyLimits(t *testing.T) {
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Upload: config.UploadConfig{
			FileBatchLimit:   5,
			UploadQueueLimit: 200,
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})

	got := (&fileService{}).GetUploadConfig()
	if got.BatchCountLimit != 5 {
		t.Fatalf("BatchCountLimit = %d, want 5", got.BatchCountLimit)
	}
	if got.UploadQueueLimit != 200 {
		t.Fatalf("UploadQueueLimit = %d, want 200", got.UploadQueueLimit)
	}
}
