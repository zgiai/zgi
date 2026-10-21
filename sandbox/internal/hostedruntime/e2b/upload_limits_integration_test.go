//go:build e2b_integration && linux

package e2b

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

type zeroUploadReader struct{}

func (zeroUploadReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

// This opt-in test creates one real disposable Sandbox. It deliberately tests
// input transfer only; it does not expand the independent result-export limit.
func TestE2BChatAttachmentUploadLimits(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_KEY"))
	template := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE"))
	if key == "" || template == "" {
		t.Fatal("real E2B credentials and an updated template are required")
	}
	sources := (packagesource.Config{}).Resolved()
	backend, err := New(Options{APIKey: key, Profiles: map[string]Profile{"upload": {TemplateID: template, DependencyInstallation: true, PackageNetworkMode: "package-install", PackageSources: sources, EgressAllowedHosts: sources.Hosts()}}, TimeoutSeconds: 300})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	allocation, err := backend.CreateBinding(ctx, agentexecution.CreateSpec{BindingID: "chat-upload-" + time.Now().Format("20060102150405"), WorkspaceID: "chat-upload-contract", ProfileID: "upload"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := backend.DestroyBinding(cleanup, allocation); err != nil {
			t.Errorf("destroy disposable Sandbox: %v", err)
		}
	})
	caps, err := backend.Capabilities(ctx, allocation)
	if err != nil || caps.MaxUploadBytes != 100<<20 || caps.MaxDownloadBytes != 64<<20 {
		t.Fatalf("capabilities=%+v error=%v", caps, err)
	}
	for _, size := range []int64{70 << 20, 100 << 20} {
		info, err := backend.UploadFile(ctx, allocation, "large-input.bin", io.LimitReader(zeroUploadReader{}, size), size)
		if err != nil || info.Size != size {
			t.Fatalf("upload %d bytes: info=%+v error=%v", size, info, err)
		}
		t.Logf("real upload accepted: %d original bytes", size)
	}
	if _, err := backend.UploadFile(ctx, allocation, "too-large.bin", io.LimitReader(zeroUploadReader{}, (100<<20)+1), (100<<20)+1); err == nil {
		t.Fatal("over-limit upload accepted")
	}
}
