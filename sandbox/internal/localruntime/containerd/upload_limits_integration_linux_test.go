//go:build linux && agent_runtime_integration

package containerd

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
)

type zeroAttachmentReader struct{}

func (zeroAttachmentReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

func TestContainerdChatAttachmentUploadLimits(t *testing.T) {
	imageRef := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if os.Geteuid() != 0 || imageRef == "" {
		t.Fatal("a root-owned real gVisor worker and an updated image are required")
	}
	// Linux UDS names include the control directory and must fit sun_path.
	root, err := os.MkdirTemp("", "zgi-upload-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	provider, err := New(Options{Address: os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS"), RootDir: root, Profiles: map[string]Profile{"upload": {Image: imageRef}}, StartupLimit: 45 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	backend, err := localruntime.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	allocation, err := backend.CreateBinding(ctx, agentexecution.CreateSpec{BindingID: "chat-upload-contract", WorkspaceID: "chat-upload-contract", ProfileID: "upload"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := backend.DestroyBinding(cleanup, allocation); err != nil {
			t.Errorf("destroy contract environment: %v", err)
		}
	})
	caps, err := backend.Capabilities(ctx, allocation)
	if err != nil || caps.MaxUploadBytes != 100<<20 || caps.MaxDownloadBytes != 64<<20 {
		t.Fatalf("capabilities=%+v error=%v", caps, err)
	}
	for _, size := range []int64{70 << 20, 100 << 20} {
		info, err := backend.UploadFile(ctx, allocation, "large-input.bin", io.LimitReader(zeroAttachmentReader{}, size), size)
		if err != nil || info.Size != size {
			t.Fatalf("upload %d bytes: info=%+v error=%v", size, info, err)
		}
		t.Logf("real gVisor upload accepted: %d bytes", size)
	}
	if _, err := backend.UploadFile(ctx, allocation, "too-large.bin", io.LimitReader(zeroAttachmentReader{}, (100<<20)+1), (100<<20)+1); err == nil {
		t.Fatal("over-limit upload accepted")
	}
	var input bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			picture.Set(x, y, color.RGBA{R: 23, G: 80, B: 190, A: 255})
		}
	}
	if err := png.Encode(&input, picture); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.UploadFile(ctx, allocation, "input.png", bytes.NewReader(input.Bytes()), int64(input.Len())); err != nil {
		t.Fatal(err)
	}
	const csv = "item,value\nalpha,12\nbeta,30\n"
	if _, err := backend.UploadFile(ctx, allocation, "input.csv", strings.NewReader(csv), int64(len(csv))); err != nil {
		t.Fatal(err)
	}
	result, output := runContractJob(t, backend, allocation, agentexecution.JobSpec{JobID: "transform-attachments", Command: "python3", Timeout: 30 * time.Second, Args: []string{"-c", "import csv,json; from PIL import Image; im=Image.open('input.png'); im.resize((60,40)).save('result.png'); total=sum(int(r['value']) for r in csv.DictReader(open('input.csv'))); open('result.json','w').write(json.dumps({'total':total})); assert total==42; print('ATTACHMENT_TRANSFORMS_OK')"}})
	if result.ExitCode != 0 || !bytes.Contains(output, []byte("ATTACHMENT_TRANSFORMS_OK")) {
		t.Fatalf("transform result=%+v output=%s", result, output)
	}
	reader, _, err := backend.DownloadFile(ctx, allocation, "result.png")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	dimensions, err := png.DecodeConfig(reader)
	if err != nil || dimensions.Width != 60 || dimensions.Height != 40 {
		t.Fatalf("downloaded image=%+v error=%v", dimensions, err)
	}
}
