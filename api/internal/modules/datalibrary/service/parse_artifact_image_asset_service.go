package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
)

const (
	documentImageAssetEndpoint      = "/console/api/files/document-images"
	documentImageAssetMaxBytes      = 20 * 1024 * 1024
	documentImageAssetDownloadDelay = 15 * time.Second
)

var (
	documentMarkdownDataURIImagePattern = regexp.MustCompile(`!\[([^\]\r\n]*)\]\((data:image/[^)\r\n]+)\)`)
	documentHTMLImageSrcPattern         = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*)(["']?)([^"'\s>]+)(["']?)`)
)

type ParseArtifactImageAssetService interface {
	Normalize(ctx context.Context, input ParseArtifactImageAssetNormalizeInput) (*ParseArtifactImageAssetNormalizeResult, error)
}

type ParseArtifactImageAssetNormalizeInput struct {
	OrganizationID string
	SourceFileID   string
	Artifact       *contracts.ParseArtifact
}

type ParseArtifactImageAssetNormalizeResult struct {
	StoredImageCount int
}

type parseArtifactImageAssetService struct {
	storage storage.Storage
}

func NewParseArtifactImageAssetService(storage storage.Storage) ParseArtifactImageAssetService {
	return &parseArtifactImageAssetService{storage: storage}
}

func (s *parseArtifactImageAssetService) Normalize(ctx context.Context, input ParseArtifactImageAssetNormalizeInput) (*ParseArtifactImageAssetNormalizeResult, error) {
	result := &ParseArtifactImageAssetNormalizeResult{}
	if input.Artifact == nil || s == nil || s.storage == nil {
		return result, nil
	}

	namespace := sanitizeDocumentImageAssetPathPart(input.OrganizationID) + "/" + sanitizeDocumentImageAssetPathPart(input.SourceFileID)
	namespace = strings.Trim(namespace, "/")
	if namespace == "" {
		namespace = "unknown"
	}

	cache := map[string]storedParseArtifactImage{}
	imageAssets := documentImageAssets(input.Artifact.Metadata)
	changed := false
	for i := range input.Artifact.Elements {
		element := &input.Artifact.Elements[i]
		stored, source, originalRef, err := s.persistElementImage(ctx, element, namespace, imageAssets, cache)
		if err != nil {
			return result, err
		}
		if stored.StorageKey != "" {
			applyElementPrimaryImage(element, stored, source, originalRef)
			element.Content = prependDocumentImageMarkdown(element.Content, stored.URL)
			changed = true
		}
		embedded, err := s.persistElementEmbeddedDataURIImages(element, namespace, cache)
		if err != nil {
			return result, err
		}
		if len(embedded) > 0 {
			if element.Metadata == nil {
				element.Metadata = map[string]any{}
			}
			element.Metadata["embedded_image_count"] = len(embedded)
			element.Metadata["embedded_images"] = embedded
			changed = true
		}
	}
	if len(imageAssets) > 0 {
		delete(input.Artifact.Metadata, "image_assets")
	}

	result.StoredImageCount = len(cache)
	if changed {
		rebuildParseArtifactAggregateContent(input.Artifact)
		if input.Artifact.Metadata == nil {
			input.Artifact.Metadata = map[string]any{}
		}
		input.Artifact.Metadata["persisted_document_image_count"] = len(cache)
	}
	return result, nil
}

func (s *parseArtifactImageAssetService) persistElementImage(
	ctx context.Context,
	element *contracts.ParsedElement,
	namespace string,
	imageAssets map[string]string,
	cache map[string]storedParseArtifactImage,
) (storedParseArtifactImage, string, string, error) {
	if isReductoFigureImageElement(element) {
		rawURL := reductoElementImageURL(element.Metadata)
		if rawURL == "" {
			return storedParseArtifactImage{}, "", "", nil
		}
		stored, err := s.persistReductoImage(ctx, rawURL, namespace, cache)
		return stored, "reducto", rawURL, err
	}
	if isMineruFigureImageElement(element) {
		if dataURI, originalRef := mineruElementImageDataURI(element.Metadata); dataURI != "" {
			stored, err := s.persistDataURIImage(originalRef, dataURI, namespace, cache)
			return stored, "mineru", originalRef, err
		}
		imagePath := mineruElementImagePath(element.Metadata)
		if imagePath == "" {
			return storedParseArtifactImage{}, "", "", nil
		}
		dataURI, ok := lookupDocumentImageAsset(imageAssets, imagePath)
		if !ok {
			return storedParseArtifactImage{}, "mineru", imagePath, fmt.Errorf("mineru image asset not found for %q", imagePath)
		}
		stored, err := s.persistDataURIImage(imagePath, dataURI, namespace, cache)
		return stored, "mineru", imagePath, err
	}
	return storedParseArtifactImage{}, "", "", nil
}

func applyElementPrimaryImage(element *contracts.ParsedElement, stored storedParseArtifactImage, source string, originalRef string) {
	if element.Metadata == nil {
		element.Metadata = map[string]any{}
	}
	if source == "reducto" {
		element.Metadata["original_image_url"] = originalRef
		element.Metadata["image_ref_type"] = "url"
	} else {
		element.Metadata["original_image_path"] = originalRef
		element.Metadata["image_ref_type"] = "data_uri"
	}
	element.Metadata["image_url"] = stored.URL
	element.Metadata["image_key"] = stored.StorageKey
	element.Metadata["image_asset_source"] = source
	if payload, ok := element.Metadata["payload"].(map[string]any); ok {
		if source == "reducto" {
			payload["original_image_url"] = originalRef
			payload["image_ref_type"] = "url"
		} else {
			payload["original_img_path"] = originalRef
			payload["img_path"] = stored.URL
			payload["image_ref_type"] = "data_uri"
			delete(payload, "image_data_uri")
		}
		payload["image_url"] = stored.URL
		payload["image_key"] = stored.StorageKey
	}
}

type storedParseArtifactImage struct {
	OriginalURL string
	StorageKey  string
	URL         string
}

type embeddedParseArtifactImage struct {
	OriginalRef string `json:"original_ref"`
	ImageURL    string `json:"image_url"`
	ImageKey    string `json:"image_key"`
}

func (s *parseArtifactImageAssetService) persistReductoImage(
	ctx context.Context,
	imageURL string,
	namespace string,
	cache map[string]storedParseArtifactImage,
) (storedParseArtifactImage, error) {
	imageURL = strings.TrimSpace(imageURL)
	if !isPersistableDocumentImageURL(imageURL) {
		return storedParseArtifactImage{}, fmt.Errorf("unsupported document image URL: %s", imageURL)
	}
	if stored, ok := cache[imageURL]; ok {
		return stored, nil
	}

	data, contentType, err := downloadDocumentImage(ctx, imageURL)
	if err != nil {
		return storedParseArtifactImage{}, fmt.Errorf("download reducto document image: %w", err)
	}

	stored, err := s.persistDocumentImageData(mustDocumentImageURLPath(imageURL), namespace, data, contentType)
	if err != nil {
		return storedParseArtifactImage{}, fmt.Errorf("save reducto document image: %w", err)
	}
	stored.OriginalURL = imageURL
	cache[imageURL] = stored
	return stored, nil
}

func (s *parseArtifactImageAssetService) persistDataURIImage(
	imagePath string,
	dataURI string,
	namespace string,
	cache map[string]storedParseArtifactImage,
) (storedParseArtifactImage, error) {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return storedParseArtifactImage{}, fmt.Errorf("document image path is empty")
	}
	cacheSum := sha256.Sum256([]byte(strings.TrimSpace(dataURI)))
	cacheKey := fmt.Sprintf("data_uri:%x", cacheSum[:])
	if stored, ok := cache[cacheKey]; ok {
		return stored, nil
	}
	data, contentType, err := decodeDocumentImageDataURI(dataURI)
	if err != nil {
		return storedParseArtifactImage{}, fmt.Errorf("decode mineru document image: %w", err)
	}
	stored, err := s.persistDocumentImageData(imagePath, namespace, data, contentType)
	if err != nil {
		return storedParseArtifactImage{}, err
	}
	cache[cacheKey] = stored
	return stored, nil
}

func (s *parseArtifactImageAssetService) persistElementEmbeddedDataURIImages(
	element *contracts.ParsedElement,
	namespace string,
	cache map[string]storedParseArtifactImage,
) ([]embeddedParseArtifactImage, error) {
	if element == nil {
		return nil, nil
	}

	seen := map[string]embeddedParseArtifactImage{}
	addEmbedded := func(images []embeddedParseArtifactImage) {
		for _, image := range images {
			if image.ImageKey == "" {
				continue
			}
			seen[image.ImageKey] = image
		}
	}

	rewritten, images, err := s.rewriteEmbeddedDataURIImages(element.Content, namespace, cache)
	if err != nil {
		return nil, err
	}
	if len(images) > 0 {
		element.Content = rewritten
		addEmbedded(images)
	}

	if element.Metadata != nil {
		if markdown, ok := element.Metadata["markdown"].(string); ok {
			rewritten, images, err := s.rewriteEmbeddedDataURIImages(markdown, namespace, cache)
			if err != nil {
				return nil, err
			}
			if len(images) > 0 {
				element.Metadata["markdown"] = rewritten
				addEmbedded(images)
			}
		}
		if payload, ok := element.Metadata["payload"].(map[string]any); ok {
			if tableBody, ok := payload["table_body"].(string); ok {
				rewritten, images, err := s.rewriteEmbeddedDataURIImages(tableBody, namespace, cache)
				if err != nil {
					return nil, err
				}
				if len(images) > 0 {
					payload["table_body"] = rewritten
					addEmbedded(images)
				}
			}
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}
	out := make([]embeddedParseArtifactImage, 0, len(seen))
	for _, image := range seen {
		out = append(out, image)
	}
	return out, nil
}

func (s *parseArtifactImageAssetService) rewriteEmbeddedDataURIImages(
	content string,
	namespace string,
	cache map[string]storedParseArtifactImage,
) (string, []embeddedParseArtifactImage, error) {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "data:image/") {
		return content, nil, nil
	}

	var embedded []embeddedParseArtifactImage
	var firstErr error
	rewritten := documentMarkdownDataURIImagePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := documentMarkdownDataURIImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		stored, err := s.persistDataURIImage("embedded-markdown-image", parts[2], namespace, cache)
		if err != nil {
			firstErr = err
			return match
		}
		embedded = append(embedded, embeddedParseArtifactImage{
			OriginalRef: "embedded-markdown-image",
			ImageURL:    stored.URL,
			ImageKey:    stored.StorageKey,
		})
		return fmt.Sprintf("![%s](%s)", parts[1], stored.URL)
	})
	if firstErr != nil {
		return content, nil, firstErr
	}

	rewritten = documentHTMLImageSrcPattern.ReplaceAllStringFunc(rewritten, func(match string) string {
		parts := documentHTMLImageSrcPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		if !isDocumentImageDataURI(parts[3]) {
			return match
		}
		stored, err := s.persistDataURIImage("embedded-html-image", parts[3], namespace, cache)
		if err != nil {
			firstErr = err
			return match
		}
		embedded = append(embedded, embeddedParseArtifactImage{
			OriginalRef: "embedded-html-image",
			ImageURL:    stored.URL,
			ImageKey:    stored.StorageKey,
		})
		quote := parts[2]
		if quote == "" {
			quote = `"`
		}
		return parts[1] + quote + stored.URL + quote
	})
	if firstErr != nil {
		return content, nil, firstErr
	}
	return rewritten, embedded, nil
}

func (s *parseArtifactImageAssetService) persistDocumentImageData(
	originalRef string,
	namespace string,
	data []byte,
	contentType string,
) (storedParseArtifactImage, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(originalRef)))
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
	if err := s.storage.Save(key, data); err != nil {
		logger.WarnContext(context.Background(), "failed to save document image", "storage_key", key, err)
		return storedParseArtifactImage{}, fmt.Errorf("save document image: %w", err)
	}

	return storedParseArtifactImage{
		OriginalURL: originalRef,
		StorageKey:  key,
		URL:         buildDocumentImageAssetURL(key),
	}, nil
}

func isReductoFigureImageElement(element *contracts.ParsedElement) bool {
	if element == nil || strings.ToLower(strings.TrimSpace(element.Type)) != "figure" {
		return false
	}
	payload, ok := element.Metadata["payload"].(map[string]any)
	if !ok {
		return false
	}
	source, _ := payload["source"].(string)
	return strings.EqualFold(strings.TrimSpace(source), "reducto")
}

func isMineruFigureImageElement(element *contracts.ParsedElement) bool {
	if element == nil || strings.ToLower(strings.TrimSpace(element.Type)) != "figure" {
		return false
	}
	payload, ok := element.Metadata["payload"].(map[string]any)
	if !ok {
		return false
	}
	mineruType, _ := payload["mineru_type"].(string)
	switch strings.ToLower(strings.TrimSpace(mineruType)) {
	case "image", "chart", "figure":
		return true
	default:
		return false
	}
}

func reductoElementImageURL(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata["image_url"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if payload, ok := metadata["payload"].(map[string]any); ok {
		if value, ok := payload["image_url"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mineruElementImagePath(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if payload, ok := metadata["payload"].(map[string]any); ok {
		if value, ok := payload["img_path"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if markdown, ok := metadata["markdown"].(string); ok {
		return extractFirstMarkdownImagePath(markdown)
	}
	return ""
}

func mineruElementImageDataURI(metadata map[string]any) (string, string) {
	if metadata == nil {
		return "", ""
	}
	payload, ok := metadata["payload"].(map[string]any)
	if !ok {
		return "", ""
	}
	originalRef := "mineru-image"
	if value, ok := payload["original_img_path"].(string); ok && strings.TrimSpace(value) != "" {
		originalRef = strings.TrimSpace(value)
	}
	if value, ok := payload["image_data_uri"].(string); ok && isDocumentImageDataURI(value) {
		return strings.TrimSpace(value), originalRef
	}
	if value, ok := payload["img_path"].(string); ok && isDocumentImageDataURI(value) {
		return strings.TrimSpace(value), originalRef
	}
	return "", ""
}

func isDocumentImageDataURI(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "data:image/") && strings.Contains(value, ";base64,")
}

func documentImageAssets(metadata map[string]any) map[string]string {
	raw, ok := metadata["image_assets"]
	if !ok || raw == nil {
		return nil
	}
	out := map[string]string{}
	switch assets := raw.(type) {
	case map[string]string:
		for key, value := range assets {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
				out[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	case map[string]any:
		for key, value := range assets {
			if text, ok := value.(string); ok && strings.TrimSpace(key) != "" && strings.TrimSpace(text) != "" {
				out[strings.TrimSpace(key)] = strings.TrimSpace(text)
			}
		}
	}
	return out
}

func lookupDocumentImageAsset(assets map[string]string, imagePath string) (string, bool) {
	if len(assets) == 0 {
		return "", false
	}
	for _, candidate := range documentImageAssetNameCandidates(imagePath) {
		if value, ok := assets[candidate]; ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func documentImageAssetNameCandidates(imagePath string) []string {
	normalized := strings.Trim(strings.ReplaceAll(imagePath, "\\", "/"), "/")
	if normalized == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	add := func(value string) {
		value = strings.Trim(strings.ReplaceAll(value, "\\", "/"), "/")
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(normalized)
	if idx := strings.Index(strings.ToLower(normalized), "images/"); idx >= 0 {
		add(normalized[idx:])
	}
	add(filepath.Base(normalized))
	return out
}

func extractFirstMarkdownImagePath(markdown string) string {
	start := strings.Index(markdown, "](")
	if start < 0 {
		return ""
	}
	remaining := markdown[start+2:]
	end := strings.Index(remaining, ")")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(remaining[:end])
}

func prependDocumentImageMarkdown(content string, imageURL string) string {
	content = strings.TrimSpace(content)
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return content
	}
	markdown := fmt.Sprintf("![figure](%s)", imageURL)
	if strings.Contains(content, markdown) {
		return content
	}
	if content == "" || content == "[figure]" {
		return markdown
	}
	return markdown + "\n\n" + content
}

func decodeDocumentImageDataURI(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, "", fmt.Errorf("empty image data")
	}
	contentType := "application/octet-stream"
	payload := value
	if strings.HasPrefix(value, "data:") {
		header, body, ok := strings.Cut(value, ",")
		if !ok {
			return nil, "", fmt.Errorf("invalid data URI")
		}
		payload = body
		meta := strings.TrimPrefix(header, "data:")
		if semi := strings.Index(meta, ";"); semi >= 0 {
			meta = meta[:semi]
		}
		if strings.TrimSpace(meta) != "" {
			contentType = strings.TrimSpace(meta)
		}
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("unsupported content type %s", contentType)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("decode base64: %w", err)
	}
	return data, contentType, nil
}

func rebuildParseArtifactAggregateContent(artifact *contracts.ParseArtifact) {
	if artifact == nil {
		return
	}
	parts := make([]string, 0, len(artifact.Elements))
	for _, element := range artifact.Elements {
		if content := strings.TrimSpace(element.Content); content != "" {
			parts = append(parts, content)
		}
	}
	if len(parts) == 0 {
		return
	}
	joined := strings.Join(parts, "\n\n")
	artifact.Markdown = joined
	artifact.Text = joined
}

func downloadDocumentImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, documentImageAssetDownloadDelay)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "zgi-document-image-ingest/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, documentImageAssetMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if len(data) > documentImageAssetMaxBytes {
		return nil, "", fmt.Errorf("image exceeds %d bytes", documentImageAssetMaxBytes)
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

func isPersistableDocumentImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func mustDocumentImageURLPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return ""
	}
	return parsed.Path
}

func buildDocumentImageAssetURL(key string) string {
	return documentImageAssetEndpoint + "?key=" + url.QueryEscape(key)
}

func sanitizeDocumentImageAssetPathPart(value string) string {
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
