package music

import (
	"net/url"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func TestToolFileAssetStoreURLRequestsDirectDelivery(t *testing.T) {
	previous := tool_file.GlobalFileSignature
	t.Cleanup(func() { tool_file.GlobalFileSignature = previous })
	tool_file.InitFileSignature(&config.Config{App: config.AppConfig{
		SecretKey: "test-secret",
		FilesURL:  "https://api.example.com",
	}})

	rawURL, err := NewToolFileAssetStore().URL(t.Context(), "file-1")
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if got := parsedURL.Query().Get("delivery"); got != "direct" {
		t.Errorf("delivery = %q, want %q", got, "direct")
	}
}
