package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/util"
)

const (
	knowledgeImageEndpoint = "/console/api/files/mineru-images"
)

var (
	knowledgeMarkdownImagePattern = regexp.MustCompile(`!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)`)
	knowledgeHTMLImageSrcPattern  = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*)(["']?)([^"'\s>]+)(["']?)`)
)

// NormalizeKnowledgeImageURLs rewrites image sources in knowledge content to public URLs.
func NormalizeKnowledgeImageURLs(content string, filesBaseURL string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return content, nil
	}

	var firstErr error
	rewritten := knowledgeMarkdownImagePattern.ReplaceAllStringFunc(content, func(match string) string {
		if firstErr != nil {
			return match
		}
		parts := knowledgeMarkdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		normalized, ok, err := normalizeKnowledgeImageSource(parts[2], filesBaseURL)
		if err != nil {
			firstErr = err
			return match
		}
		if !ok {
			return match
		}
		return fmt.Sprintf("![%s](%s)", parts[1], normalized)
	})
	if firstErr != nil {
		return "", firstErr
	}

	rewritten = knowledgeHTMLImageSrcPattern.ReplaceAllStringFunc(rewritten, func(match string) string {
		if firstErr != nil {
			return match
		}
		parts := knowledgeHTMLImageSrcPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		normalized, ok, err := normalizeKnowledgeImageSource(parts[3], filesBaseURL)
		if err != nil {
			firstErr = err
			return match
		}
		if !ok {
			return match
		}
		quote := parts[2]
		if quote == "" {
			quote = `"`
		}
		return parts[1] + quote + normalized + quote
	})
	if firstErr != nil {
		return "", firstErr
	}

	return rewritten, nil
}

func normalizeKnowledgeImageSource(rawSource string, filesBaseURL string) (string, bool, error) {
	source := stripKnowledgeMarkdownLinkBrackets(rawSource)
	if source == "" {
		return "", false, nil
	}

	if isLocalMinerUImagePath(source) {
		return buildKnowledgeImageProxyURL(source, filesBaseURL)
	}

	parsed, err := url.Parse(source)
	if err != nil || parsed == nil {
		return "", false, nil
	}
	if shouldPreserveImageSource(parsed) {
		return "", false, nil
	}

	baseURL, err := normalizeKnowledgeFilesBaseURL(filesBaseURL)
	if err != nil {
		return "", true, err
	}

	return joinKnowledgeFilesBaseURL(baseURL, parsed.String()), true, nil
}

func normalizeKnowledgeFilesBaseURL(filesBaseURL string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(filesBaseURL), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid files url for knowledge image urls: %q", filesBaseURL)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid files url for knowledge image urls: %q", filesBaseURL)
	}
	return value, nil
}

func shouldPreserveImageSource(parsed *url.URL) bool {
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "" {
		return true
	}
	return parsed.Host != ""
}

func buildKnowledgeImageProxyURL(localPath string, filesBaseURL string) (string, bool, error) {
	baseURL, err := normalizeKnowledgeFilesBaseURL(filesBaseURL)
	if err != nil {
		return "", true, err
	}
	return baseURL + util.GetSignedParserImagePathURL(localPath), true, nil
}

func joinKnowledgeFilesBaseURL(baseURL string, source string) string {
	if strings.HasPrefix(source, "/") {
		return baseURL + source
	}
	return baseURL + "/" + source
}

func isLocalMinerUImagePath(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
	return strings.Contains(normalized, "/mineru/images/") ||
		strings.Contains(normalized, "/hyperparse/mineru/images/") ||
		strings.Contains(normalized, "/storage/mineru/images/")
}

func stripKnowledgeMarkdownLinkBrackets(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">"))
	}
	return value
}
