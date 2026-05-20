package file

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/zgiai/ginext/config"
	toolfile "github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/ginext/internal/util"
)

func GetSignedFileURL(uploadFileID string) (string, error) {
	return util.GetSignedFileURL(uploadFileID)
}

func SignToolFile(toolFileID, extension string) (string, error) {
	return SignToolFileWithMode(toolFileID, extension, toolfile.ToolFileURLModeSigned)
}

func VerifyToolFileSignature(toolFileID, timestamp, nonce, sign string) bool {
	return newToolFileSignature().VerifyToolFileSignature(toolFileID, timestamp, nonce, sign)
}

func SignToolFileWithMode(toolFileID, extension string, mode toolfile.ToolFileURLMode) (string, error) {
	return newToolFileSignature().SignToolFileWithMode(toolFileID, extension, mode)
}

func VerifyToolFileSignatureWithExpiry(toolFileID, expiresAt, nonce, sign string) bool {
	return newToolFileSignature().VerifyToolFileSignatureWithExpiry(toolFileID, expiresAt, nonce, sign)
}

func newToolFileSignature() *toolfile.FileSignature {
	cfg := config.GlobalConfig
	if cfg == nil {
		cfg = config.Current()
	}

	return toolfile.NewFileSignature(cfg)
}

func generateSignature(data, secretKey string) (string, error) {
	h := hmac.New(sha256.New, []byte(secretKey))
	_, err := h.Write([]byte(data))
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(h.Sum(nil)), nil
}

type ExternalURLInfo struct {
	Scheme   string
	Host     string
	IsPublic bool
}

func InspectExternalURL(rawURL string) (*ExternalURLInfo, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("file URL is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("file URL is invalid: %w", err)
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("file URL must be absolute")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("file URL must use http or https")
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return nil, fmt.Errorf("file URL host is required")
	}

	return &ExternalURLInfo{
		Scheme:   scheme,
		Host:     host,
		IsPublic: isPublicURLHost(host),
	}, nil
}

func IsDevelopmentEnvironment() bool {
	cfg := config.Current()
	return strings.EqualFold(cfg.Server.Mode, "debug") ||
		strings.EqualFold(cfg.Server.Environment, "local") ||
		strings.EqualFold(cfg.Server.Environment, "dev")
}

func isPublicURLHost(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return false
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		return isPublicAddr(addr)
	}
	if ip := net.ParseIP(host); ip != nil {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			return isPublicAddr(addr)
		}
		return false
	}

	return strings.Contains(host, ".")
}

func isPublicAddr(addr netip.Addr) bool {
	return !addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

// GetOSSPublicURL generates a public OSS URL for the given storage key.
// The key should be in format: upload_files/{tenant_id}/{file_id}.{ext}
// Returns URL in format: https://{bucket}.{endpoint}/{path}/{key}
func GetOSSPublicURL(storageKey string) string {
	oss := config.Current().Storage.AliyunOSS
	bucket := oss.BucketName
	if bucket == "" {
		bucket = "zgi"
	}
	endpoint := oss.Endpoint
	if endpoint == "" {
		endpoint = "oss-cn-beijing.aliyuncs.com"
	}
	path := oss.Folder
	if path == "" {
		path = "test"
	}

	// Build full URL
	if path != "" {
		return fmt.Sprintf("https://%s.%s/%s/%s", bucket, endpoint, path, storageKey)
	}
	return fmt.Sprintf("https://%s.%s/%s", bucket, endpoint, storageKey)
}
