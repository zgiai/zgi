package imageasset

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	defaultImageMIME                  = "image/png"
	defaultImageExt                   = ".png"
	maxImageBytes                     = 20 * 1024 * 1024
	generatedImageDownloadAttempts    = 3
	generatedImageDownloadTimeout     = 120 * time.Second
	generatedImageTLSHandshakeTimeout = 30 * time.Second
	generatedImageResponseTimeout     = 60 * time.Second
	generatedImageMaxRedirects        = 5
)

var unsafeFilenamePattern = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)

type SaveRequest struct {
	TenantID       string
	UserID         string
	ConversationID *string
	Item           adapter.ImageItem
	BaseFilename   string
	Index          int
	Lifecycle      tool_file.ToolFileLifecycle
}

type Service interface {
	SaveGeneratedImage(ctx context.Context, req SaveRequest) (map[string]interface{}, error)
	DeleteGeneratedImage(ctx context.Context, fileID string) error
}

type service struct{}

func NewService() Service {
	return service{}
}

func SaveGeneratedImage(ctx context.Context, req SaveRequest) (map[string]interface{}, error) {
	return NewService().SaveGeneratedImage(ctx, req)
}

func (service) SaveGeneratedImage(ctx context.Context, req SaveRequest) (map[string]interface{}, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	userID := strings.TrimSpace(req.UserID)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant id is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id is required")
	}
	lifecycle := req.Lifecycle
	if lifecycle == "" {
		lifecycle = tool_file.ToolFileLifecyclePersistent
	}

	var toolFile *tool_file.ToolFile
	var err error
	switch {
	case strings.TrimSpace(req.Item.B64JSON) != "":
		data, decodeErr := decodeBase64Image(req.Item.B64JSON)
		if decodeErr != nil {
			return nil, decodeErr
		}
		_, mimeType, extension, validateErr := validateGeneratedImageData(data, "")
		if validateErr != nil {
			return nil, validateErr
		}
		filename := buildImageFilename(req.BaseFilename, req.Index, extension)
		toolFile, err = tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
			UserID:         userID,
			TenantID:       tenantID,
			ConversationID: req.ConversationID,
			FileData:       data,
			MimeType:       mimeType,
			Filename:       &filename,
			Lifecycle:      lifecycle,
		})
	case strings.TrimSpace(req.Item.URL) != "":
		data, mimeType, extension, downloadErr := downloadGeneratedImage(ctx, strings.TrimSpace(req.Item.URL))
		if downloadErr != nil {
			return nil, downloadErr
		}
		filename := buildImageFilename(req.BaseFilename, req.Index, extension)
		toolFile, err = tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
			UserID:         userID,
			TenantID:       tenantID,
			ConversationID: req.ConversationID,
			FileData:       data,
			MimeType:       mimeType,
			Filename:       &filename,
			Lifecycle:      lifecycle,
		})
	default:
		return nil, fmt.Errorf("image item does not contain url or b64_json")
	}
	if err != nil {
		return nil, err
	}

	extension := toolFile.GetFileExtension()
	if extension == "" {
		extension = extensionFromMIME(toolFile.MimeType)
	}
	if extension == "" {
		extension = defaultImageExt
	}
	url, err := tool_file.SignToolFileGlobal(toolFile.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to sign generated image: %w", err)
	}
	downloadURL := appendDownloadQuery(url)
	mimeType := strings.TrimSpace(toolFile.MimeType)
	if mimeType == "" {
		mimeType = defaultImageMIME
	}
	fileObj := workflowfile.NewFile(
		tenantID,
		workflowfile.FileTypeImage,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(extension),
		workflowfile.WithMimeType(mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["file_id"] = toolFile.ID
	fileMeta["tool_file_id"] = toolFile.ID
	fileMeta["filename"] = toolFile.Name
	fileMeta["extension"] = extension
	fileMeta["format"] = strings.TrimPrefix(extension, ".")
	fileMeta["mime_type"] = mimeType
	fileMeta["transfer_method"] = string(workflowfile.FileTransferMethodToolFile)
	fileMeta["lifecycle"] = string(toolFile.LifecycleValue())
	if toolFile.ExpiresAt != nil {
		fileMeta["expires_at"] = toolFile.ExpiresAt.Unix()
	}
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL
	return fileMeta, nil
}

func (service) DeleteGeneratedImage(ctx context.Context, fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return fmt.Errorf("file id is required")
	}
	return tool_file.DeleteToolFileGlobal(ctx, fileID)
}

func decodeBase64Image(raw string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		return data, nil
	}
	data, rawErr := base64.RawStdEncoding.DecodeString(raw)
	if rawErr == nil {
		return data, nil
	}
	return nil, fmt.Errorf("failed to decode image base64: %w", err)
}

func downloadGeneratedImage(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	client := generatedImageDownloadClient()
	var lastErr error
	for attempt := 1; attempt <= generatedImageDownloadAttempts; attempt++ {
		data, mimeType, extension, err := downloadGeneratedImageOnce(ctx, client, rawURL)
		if err == nil {
			return data, mimeType, extension, nil
		}
		lastErr = err
		if !shouldRetryGeneratedImageDownload(err) || attempt == generatedImageDownloadAttempts {
			break
		}
		delay := time.Duration(attempt) * 500 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, "", "", ctx.Err()
		case <-timer.C:
		}
	}
	return nil, "", "", lastErr
}

func generatedImageDownloadClient() *http.Client {
	return &http.Client{
		Timeout: generatedImageDownloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= generatedImageMaxRedirects {
				return fmt.Errorf("generated image download exceeded %d redirects", generatedImageMaxRedirects)
			}
			if err := validateGeneratedImageDownloadURL(req.Context(), req.URL); err != nil {
				return err
			}
			return nil
		},
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           safeGeneratedImageDialContext,
			TLSHandshakeTimeout:   generatedImageTLSHandshakeTimeout,
			ResponseHeaderTimeout: generatedImageResponseTimeout,
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
			TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
	}
}

func downloadGeneratedImageOnce(ctx context.Context, client *http.Client, rawURL string) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create generated image download request: %w", err)
	}
	if err := validateGeneratedImageDownloadURL(ctx, req.URL); err != nil {
		return nil, "", "", fmt.Errorf("unsafe generated image url: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to download generated image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := generatedImageDownloadStatusError{statusCode: resp.StatusCode}
		return nil, "", "", fmt.Errorf("failed to download generated image: %w", err)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read generated image: %w", err)
	}
	return validateGeneratedImageData(data, resp.Header.Get("Content-Type"))
}

func validateGeneratedImageDownloadURL(ctx context.Context, parsed *url.URL) error {
	if parsed == nil {
		return fmt.Errorf("url is required")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url must use http or https")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	_, err := resolvePublicGeneratedImageHost(ctx, host)
	return err
}

func safeGeneratedImageDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid generated image download address: %w", err)
	}
	addrs, err := resolvePublicGeneratedImageHost(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{}
	var lastErr error
	for _, addr := range addrs {
		target := net.JoinHostPort(addr.String(), port)
		conn, err := dialer.DialContext(ctx, network, target)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("generated image url host resolved no usable addresses")
}

func resolvePublicGeneratedImageHost(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("url host is required")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if !isPublicGeneratedImageAddr(addr) {
			return nil, fmt.Errorf("url host resolves to a non-public address")
		}
		return []netip.Addr{addr}, nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil, fmt.Errorf("url host resolves to a non-public address")
	}
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve generated image url host: %w", err)
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("generated image url host resolved no addresses")
	}
	for idx, addr := range resolved {
		addr = addr.Unmap()
		if !isPublicGeneratedImageAddr(addr) {
			return nil, fmt.Errorf("url host resolves to a non-public address")
		}
		resolved[idx] = addr
	}
	return resolved, nil
}

func isPublicGeneratedImageAddr(addr netip.Addr) bool {
	return addr.IsValid() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

type generatedImageDownloadStatusError struct {
	statusCode int
}

func (e generatedImageDownloadStatusError) Error() string {
	return fmt.Sprintf("status %d", e.statusCode)
}

func shouldRetryGeneratedImageDownload(err error) bool {
	if err == nil {
		return false
	}
	var statusErr generatedImageDownloadStatusError
	if errors.As(err, &statusErr) {
		return statusErr.statusCode >= http.StatusInternalServerError
	}
	return strings.Contains(err.Error(), "failed to download generated image") ||
		strings.Contains(err.Error(), "failed to read generated image")
}

func validateGeneratedImageData(data []byte, rawContentType string) ([]byte, string, string, error) {
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("generated image is empty")
	}
	if len(data) > maxImageBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxImageBytes)
	}

	headerMIME := ""
	if rawContentType != "" {
		if parsed, _, err := mime.ParseMediaType(rawContentType); err == nil {
			headerMIME = strings.ToLower(strings.TrimSpace(parsed))
		}
	}
	detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if isSupportedImageMIME(detected) {
		return data, detected, extensionFromMIME(detected), nil
	}
	if isSupportedImageMIME(headerMIME) && detected == "application/octet-stream" {
		return data, headerMIME, extensionFromMIME(headerMIME), nil
	}
	return nil, "", "", fmt.Errorf("generated result is not a supported image: detected=%s content_type=%s", detected, headerMIME)
}

func isSupportedImageMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func buildImageFilename(raw string, index int, extension string) string {
	name := sanitizeFilename(raw)
	if name == "" {
		name = "generated-image"
	}
	if index > 0 {
		name = fmt.Sprintf("%s-%d", name, index+1)
	}
	currentExt := filepath.Ext(name)
	if currentExt != "" {
		name = strings.TrimSuffix(name, currentExt)
	}
	return name + extension
}

func sanitizeFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = unsafeFilenamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func extensionFromMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func appendDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}
