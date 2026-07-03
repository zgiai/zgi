package extractor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	documentImageEndpoint      = "/console/api/files/mineru-images"
	documentImageMaxBytes      = 20 * 1024 * 1024
	documentImageDownloadDelay = 15 * time.Second
)

var (
	documentMarkdownImagePattern = regexp.MustCompile(`!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)`)
	documentHTMLImageSrcPattern  = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*)(["']?)([^"'\s>]+)(["']?)`)
)

type storedDocumentImage struct {
	OriginalURL string
	StorageKey  string
	URL         string
}

func (p *ExtractProcessor) persistMarkdownImageAssets(ctx context.Context, output *dto.ExtractOutput, uploadFile *model.UploadFile) *dto.ExtractOutput {
	if p == nil || p.storage == nil || output == nil {
		return output
	}

	namespace := mineruAssetNamespace(uploadFile)
	if namespace == "" {
		namespace = "unknown"
	}
	namespace = sanitizeDocumentImagePathPart(namespace)

	cache := map[string]storedDocumentImage{}
	changed := false

	rewrite := func(markdown string) string {
		rewritten, didChange := p.rewriteMarkdownImageAssets(ctx, markdown, namespace, cache)
		if didChange {
			changed = true
		}
		return rewritten
	}

	output.Markdown = rewrite(output.Markdown)
	for i := range output.Elements {
		output.Elements[i].Content = rewrite(output.Elements[i].Content)
	}

	if changed {
		if output.Metadata == nil {
			output.Metadata = map[string]any{}
		}
		output.Metadata["persisted_markdown_image_count"] = len(cache)
	}

	return output
}

func (p *ExtractProcessor) rewriteMarkdownImageAssets(
	ctx context.Context,
	markdown string,
	namespace string,
	cache map[string]storedDocumentImage,
) (string, bool) {
	if strings.TrimSpace(markdown) == "" {
		return markdown, false
	}

	changed := false
	rewritten := documentMarkdownImagePattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := documentMarkdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		stored, ok := p.persistExternalMarkdownImage(ctx, parts[2], namespace, cache)
		if !ok {
			return match
		}
		changed = true
		return fmt.Sprintf("![%s](%s)", parts[1], stored.URL)
	})

	rewritten = documentHTMLImageSrcPattern.ReplaceAllStringFunc(rewritten, func(match string) string {
		parts := documentHTMLImageSrcPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		stored, ok := p.persistExternalMarkdownImage(ctx, parts[3], namespace, cache)
		if !ok {
			return match
		}
		changed = true
		quote := parts[2]
		if quote == "" {
			quote = `"`
		}
		return parts[1] + quote + stored.URL + quote
	})

	return rewritten, changed
}

func (p *ExtractProcessor) persistExternalMarkdownImage(
	ctx context.Context,
	rawURL string,
	namespace string,
	cache map[string]storedDocumentImage,
) (storedDocumentImage, bool) {
	imageURL := stripDocumentMarkdownLinkBrackets(rawURL)
	if !isPersistableExternalImageURL(imageURL) {
		return storedDocumentImage{}, false
	}
	if stored, ok := cache[imageURL]; ok {
		return stored, true
	}

	data, contentType, err := downloadExternalImage(ctx, imageURL)
	if err != nil {
		logger.WarnContext(ctx, "failed to persist external markdown image", "image_url", imageURL, err)
		return storedDocumentImage{}, false
	}

	ext := strings.ToLower(filepath.Ext(mustURLPath(imageURL)))
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = ".img"
	}

	sum := sha256.Sum256(data)
	key := fmt.Sprintf("document-images/%s/%x%s", namespace, sum[:], ext)
	if err := p.storage.Save(key, data); err != nil {
		logger.WarnContext(ctx, "failed to save external markdown image", "image_url", imageURL, "storage_key", key, err)
		return storedDocumentImage{}, false
	}

	stored := storedDocumentImage{
		OriginalURL: imageURL,
		StorageKey:  key,
		URL:         buildDocumentImageAssetURL(key),
	}
	cache[imageURL] = stored
	return stored, true
}

func downloadExternalImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, documentImageDownloadDelay)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "zgi-image-ingest/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, documentImageMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if len(data) > documentImageMaxBytes {
		return nil, "", fmt.Errorf("image exceeds %d bytes", documentImageMaxBytes)
	}

	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		detected := http.DetectContentType(bytes.TrimSpace(data))
		if !strings.HasPrefix(detected, "image/") {
			return nil, "", fmt.Errorf("unsupported content type %s", contentType)
		}
		contentType = detected
	}

	return data, contentType, nil
}

func isPersistableExternalImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func stripDocumentMarkdownLinkBrackets(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">"))
	}
	return value
}

func mustURLPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return ""
	}
	return parsed.Path
}

func buildDocumentImageAssetURL(key string) string {
	return documentImageEndpoint + "?key=" + url.QueryEscape(key)
}

func sanitizeDocumentImagePathPart(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		part = strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return r
			case r >= 'A' && r <= 'Z':
				return r
			case r >= '0' && r <= '9':
				return r
			case r == '-', r == '_':
				return r
			default:
				return '-'
			}
		}, part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "/")
}
