package shortlink

import (
	"fmt"
	"net/url"
	"strings"
)

func normalizeTargetKind(kind string) string {
	return strings.TrimSpace(kind)
}

func validateTarget(kind, path string) error {
	kind = normalizeTargetKind(kind)
	path = strings.TrimSpace(path)
	if kind == "" {
		return fmt.Errorf("target_kind is required")
	}
	if path == "" {
		return fmt.Errorf("target_path is required")
	}
	if strings.HasPrefix(path, "//") {
		return fmt.Errorf("target_path must be an internal relative path")
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("parse target_path: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return fmt.Errorf("target_path must be an internal relative path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("target_path must not include query or fragment")
	}

	targetToken, err := targetTokenFromPath(kind, parsed.Path)
	if err != nil {
		return err
	}
	if targetToken == "" || strings.Contains(targetToken, "/") {
		return fmt.Errorf("target token path segment is required")
	}
	return nil
}

func targetTokenFromPath(kind, path string) (string, error) {
	switch kind {
	case TargetKindApprovalForm:
		if !strings.HasPrefix(path, "/a/") {
			return "", fmt.Errorf("approval short links must target /a/{token}")
		}
	case TargetKindWorkflowAnnouncement:
		if !strings.HasPrefix(path, "/n/") {
			return "", fmt.Errorf("announcement short links must target /n/{token}")
		}
	default:
		return "", fmt.Errorf("unsupported target_kind: %s", kind)
	}
	return strings.TrimSpace(strings.TrimPrefix(path, pathPrefixForKind(kind))), nil
}

func pathPrefixForKind(kind string) string {
	switch kind {
	case TargetKindApprovalForm:
		return "/a/"
	case TargetKindWorkflowAnnouncement:
		return "/n/"
	default:
		return ""
	}
}
