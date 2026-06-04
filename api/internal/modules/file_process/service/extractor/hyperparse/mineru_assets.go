package hyperparse

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"mime"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	"github.com/zgiai/zgi/api/pkg/storage"
)

const mineruImageEndpoint = "/console/api/files/mineru-images"

var markdownImagePattern = regexp.MustCompile(`!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)`)
var htmlImageSrcPattern = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*)(["']?)([^"'\s>]+)(["']?)`)

type storedMinerUImage struct {
	OriginalName string
	StorageKey   string
	URL          string
}

func persistMinerUImages(store storage.Storage, result *extractcommon.DocumentResult, namespace string) error {
	if store == nil || result == nil || len(result.ImageAssets) == 0 {
		return nil
	}

	if strings.TrimSpace(namespace) == "" {
		namespace = result.DocID
	}
	namespace = sanitizeStoragePathPart(namespace)
	if namespace == "" {
		namespace = "unknown"
	}

	byName := make(map[string]storedMinerUImage, len(result.ImageAssets))
	for name, dataURI := range result.ImageAssets {
		data, contentType, err := decodeImageDataURI(dataURI)
		if err != nil {
			return fmt.Errorf("mineru image %s: %w", name, err)
		}

		ext := strings.ToLower(filepath.Ext(name))
		if ext == "" {
			exts, _ := mime.ExtensionsByType(contentType)
			if len(exts) > 0 {
				ext = exts[0]
			}
		}
		if ext == "" {
			ext = ".img"
		}

		sum := sha256.Sum256(data)
		objectName := fmt.Sprintf("%x%s", sum[:], ext)
		key := "mineru/images/" + namespace + "/" + objectName
		if err := store.Save(key, data); err != nil {
			return fmt.Errorf("save mineru image %s: %w", name, err)
		}

		stored := storedMinerUImage{
			OriginalName: name,
			StorageKey:   key,
			URL:          buildMinerUImageAssetURL(key),
		}
		byName[name] = stored
		if base := filepath.Base(strings.ReplaceAll(name, "\\", "/")); base != "" {
			byName[base] = stored
		}
	}

	rewriteMinerUImageReferences(result, byName)
	result.ImageAssets = nil
	return nil
}

func decodeImageDataURI(value string) ([]byte, string, error) {
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

func rewriteMinerUImageReferences(result *extractcommon.DocumentResult, byName map[string]storedMinerUImage) {
	if result == nil || len(byName) == 0 {
		return
	}

	result.Markdown = rewriteMarkdownMinerUImages(result.Markdown, byName)
	for i := range result.Chunks {
		chunk := &result.Chunks[i]
		chunk.Markdown = rewriteMarkdownMinerUImages(chunk.Markdown, byName)
		if len(chunk.Payload) == 0 {
			continue
		}
		rawPath, _ := chunk.Payload["img_path"].(string)
		if stored, ok := lookupStoredMinerUImage(rawPath, byName); ok {
			chunk.Payload["original_img_path"] = rawPath
			chunk.Payload["image_key"] = stored.StorageKey
			chunk.Payload["image_url"] = stored.URL
			chunk.Payload["img_path"] = stored.URL
		}
	}
}

func rewriteMarkdownMinerUImages(markdown string, byName map[string]storedMinerUImage) string {
	if strings.TrimSpace(markdown) == "" || len(byName) == 0 {
		return markdown
	}
	rewritten := markdownImagePattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := markdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		if stored, ok := lookupStoredMinerUImage(parts[2], byName); ok {
			return fmt.Sprintf("![%s](%s)", parts[1], stored.URL)
		}
		return match
	})
	return htmlImageSrcPattern.ReplaceAllStringFunc(rewritten, func(match string) string {
		parts := htmlImageSrcPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		if stored, ok := lookupStoredMinerUImage(parts[3], byName); ok {
			quote := parts[2]
			if quote == "" {
				quote = `"`
			}
			return parts[1] + quote + stored.URL + quote
		}
		return match
	})
}

func lookupStoredMinerUImage(path string, byName map[string]storedMinerUImage) (storedMinerUImage, bool) {
	value := stripMarkdownLinkBrackets(path)
	if value == "" {
		return storedMinerUImage{}, false
	}
	if stored, ok := byName[value]; ok {
		return stored, true
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if stored, ok := byName[normalized]; ok {
		return stored, true
	}
	base := filepath.Base(normalized)
	stored, ok := byName[base]
	return stored, ok
}

func stripMarkdownLinkBrackets(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">"))
	}
	return value
}

func buildMinerUImageAssetURL(key string) string {
	return mineruImageEndpoint + "?key=" + url.QueryEscape(key)
}

func sanitizeStoragePathPart(value string) string {
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
