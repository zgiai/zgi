package file

import (
	"strings"
	"testing"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/storage"
)

func TestBuildWorkflowImageInputPublicStorageURL_AliyunOSS(t *testing.T) {
	cfg := &appconfig.Config{
		Storage: appconfig.StorageConfig{
			Type: string(storage.StorageTypeAliyunOSS),
			AliyunOSS: appconfig.AliyunOSSStorageConfig{
				BucketName: "zgi-files",
				Endpoint:   "oss-cn-hangzhou.aliyuncs.com",
				Folder:     "prod",
			},
		},
	}

	got, err := BuildWorkflowImageInputPublicStorageURL(cfg, "", "upload_files/org-1/image.jpg")
	if err != nil {
		t.Fatalf("BuildWorkflowImageInputPublicStorageURL returned error: %v", err)
	}

	want := "https://zgi-files.oss-cn-hangzhou.aliyuncs.com/prod/upload_files/org-1/image.jpg"
	if got != want {
		t.Fatalf("public URL = %q, want %q", got, want)
	}
}

func TestBuildWorkflowImageInputPublicStorageURL_Qiniu(t *testing.T) {
	cfg := &appconfig.Config{
		Storage: appconfig.StorageConfig{
			Type: string(storage.StorageTypeQiniu),
			Qiniu: appconfig.QiniuStorageConfig{
				Domain:   "cdn-qiniu.example.com",
				UseHTTPS: true,
				Folder:   "workflow",
			},
		},
	}

	got, err := BuildWorkflowImageInputPublicStorageURL(cfg, "", "upload_files/org-1/image.png")
	if err != nil {
		t.Fatalf("BuildWorkflowImageInputPublicStorageURL returned error: %v", err)
	}

	want := "https://cdn-qiniu.example.com/workflow/upload_files/org-1/image.png"
	if got != want {
		t.Fatalf("public URL = %q, want %q", got, want)
	}
}

func TestBuildWorkflowImageInputPublicStorageURL_PublicBaseURLWins(t *testing.T) {
	cfg := &appconfig.Config{
		Workflow: appconfig.WorkflowConfig{
			ImageInputPublicBaseURL: "https://cdn.example.com/assets/",
		},
		Storage: appconfig.StorageConfig{
			Type: string(storage.StorageTypeS3),
			S3: appconfig.S3StorageConfig{
				Folder: "/s3-prefix/",
			},
		},
	}

	got, err := BuildWorkflowImageInputPublicStorageURL(cfg, "", "upload_files/org-1/image.webp")
	if err != nil {
		t.Fatalf("BuildWorkflowImageInputPublicStorageURL returned error: %v", err)
	}

	want := "https://cdn.example.com/assets/s3-prefix/upload_files/org-1/image.webp"
	if got != want {
		t.Fatalf("public URL = %q, want %q", got, want)
	}
}

func TestBuildWorkflowImageInputPublicStorageURL_RequiresPublicBaseURLForS3(t *testing.T) {
	cfg := &appconfig.Config{
		Storage: appconfig.StorageConfig{
			Type: string(storage.StorageTypeS3),
		},
	}

	_, err := BuildWorkflowImageInputPublicStorageURL(cfg, "", "upload_files/org-1/image.jpg")
	if err == nil {
		t.Fatalf("expected missing public base URL error")
	}
	if !strings.Contains(err.Error(), "WORKFLOW_IMAGE_INPUT_PUBLIC_BASE_URL") {
		t.Fatalf("expected public base URL error, got %v", err)
	}
}

func TestIsWorkflowImageInputFile(t *testing.T) {
	if !IsWorkflowImageInputFile("jpg", "") {
		t.Fatalf("expected jpg to be treated as image")
	}
	if !IsWorkflowImageInputFile("", "image/heic") {
		t.Fatalf("expected image MIME type to be treated as image")
	}
	if IsWorkflowImageInputFile("pdf", "application/pdf") {
		t.Fatalf("expected pdf to be treated as non-image")
	}
}
