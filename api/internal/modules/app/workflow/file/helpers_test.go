package file

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestGetSignedFileURL_GeneratesVerifiableConsoleAPIURL(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		Console: appconfig.ConsoleConfig{
			APIURL: "https://api.zgi.im",
		},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	signedURL, err := GetSignedFileURL("file-1")
	if err != nil {
		t.Fatalf("GetSignedFileURL returned error: %v", err)
	}

	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; !strings.HasPrefix(got, "https://api.zgi.im/console/api/files/file-1/file-preview") {
		t.Fatalf("unexpected signed URL path: %s", got)
	}

	query := parsed.Query()
	if !util.VerifyFileSignature("file-1", query.Get("timestamp"), query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("expected generated signed URL to pass file preview signature verification")
	}
}

func TestSignToolFile_GeneratesVerifiableConsoleAPIURL(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		Console: appconfig.ConsoleConfig{
			APIURL: "https://api.zgi.im",
		},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	signedURL, err := SignToolFile("tool-file-1", ".png")
	if err != nil {
		t.Fatalf("SignToolFile returned error: %v", err)
	}

	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; !strings.HasPrefix(got, "https://api.zgi.im/console/api/files/tools/tool-file-1.png") {
		t.Fatalf("unexpected signed URL path: %s", got)
	}

	query := parsed.Query()
	if query.Get("expires_at") == "" || query.Get("expires_at") == "0" {
		t.Fatalf("expires_at = %q, want future timestamp", query.Get("expires_at"))
	}
	if !VerifyToolFileSignatureWithExpiry("tool-file-1", query.Get("expires_at"), query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("expected generated signed URL to pass tool file signature verification")
	}
}

func TestSignToolFileWithMode_GeneratesPermanentConsoleAPIURL(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		Console: appconfig.ConsoleConfig{
			APIURL: "https://api.zgi.im",
		},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	signedURL, err := SignToolFileWithMode("tool-file-1", ".png", toolfile.ToolFileURLModePermanent)
	if err != nil {
		t.Fatalf("SignToolFileWithMode returned error: %v", err)
	}

	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Query().Get("expires_at") != "0" {
		t.Fatalf("expires_at = %q, want %q", parsed.Query().Get("expires_at"), "0")
	}
	if !VerifyToolFileSignatureWithExpiry("tool-file-1", queryValue(parsed, "expires_at"), queryValue(parsed, "nonce"), queryValue(parsed, "sign")) {
		t.Fatalf("expected permanent tool file URL to pass tool file signature verification")
	}
}

func TestVerifyToolFileSignatureWithExpiry_RejectsTamperedExpiry(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	signedURL, err := SignToolFile("tool-file-1", ".png")
	if err != nil {
		t.Fatalf("SignToolFile returned error: %v", err)
	}

	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	query := parsed.Query()
	if !VerifyToolFileSignatureWithExpiry("tool-file-1", query.Get("expires_at"), query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("expected generated signed URL to pass tool file signature verification")
	}
	if VerifyToolFileSignatureWithExpiry("tool-file-1", "0", query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("expected tampered expires_at=0 to fail tool file signature verification")
	}
}

func TestVerifyToolFileSignatureWithExpiry_Expired(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		App: appconfig.AppConfig{
			FilesURL:           "https://api.zgi.im",
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	expiresAt := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	signature, err := generateSignature("tool-file|tool-file-1|"+expiresAt+"|nonce-expired", "test-secret")
	if err != nil {
		t.Fatalf("generateSignature returned error: %v", err)
	}
	if VerifyToolFileSignatureWithExpiry("tool-file-1", expiresAt, "nonce-expired", signature) {
		t.Fatalf("expected expired tool file signature verification to fail")
	}
}

func queryValue(parsed *url.URL, key string) string {
	return parsed.Query().Get(key)
}

func TestGenerateSignature_UsesURLSafeBase64(t *testing.T) {
	secret := "test-secret"
	for i := 0; i < 5000; i++ {
		data := "file-preview|file-1|1234567890|nonce-" + strconv.Itoa(i)
		h := hmac.New(sha256.New, []byte(secret))
		h.Write([]byte(data))
		sum := h.Sum(nil)

		want := base64.URLEncoding.EncodeToString(sum)
		std := base64.StdEncoding.EncodeToString(sum)
		if want == std {
			continue
		}

		got, err := generateSignature(data, secret)
		if err != nil {
			t.Fatalf("generateSignature returned error: %v", err)
		}
		if got != want {
			t.Fatalf("expected URL-safe base64 signature %q, got %q", want, got)
		}
		return
	}

	t.Fatalf("no deterministic candidate produced a URL-safe/base64 mismatch")
}
