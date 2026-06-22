package keys

import (
	"net/url"
	"strings"
)

const (
	// DefaultGlobalPrefix is the global namespace for application-owned cache keys.
	DefaultGlobalPrefix = "zgi_cache"
	EmptyPart           = "_"
)

type Builder struct {
	globalPrefix string
}

func NewBuilder(globalPrefix string) Builder {
	globalPrefix = normalizePart(globalPrefix)
	if globalPrefix == "" || globalPrefix == EmptyPart {
		globalPrefix = DefaultGlobalPrefix
	}
	return Builder{globalPrefix: globalPrefix}
}

func DefaultBuilder() Builder {
	return NewBuilder(DefaultGlobalPrefix)
}

func (b Builder) GlobalPrefix() string {
	if b.globalPrefix == "" {
		return DefaultGlobalPrefix
	}
	return b.globalPrefix
}

func (b Builder) Build(modulePrefix string, parts ...string) string {
	segments := []string{b.GlobalPrefix()}
	module := NormalizeModulePrefix(modulePrefix)
	if module != "" {
		segments = append(segments, strings.Split(module, ":")...)
	}
	for _, part := range parts {
		segments = append(segments, normalizePart(part))
	}
	return strings.Join(segments, ":")
}

func (b Builder) Prefix(modulePrefix string, parts ...string) string {
	return b.Build(modulePrefix, parts...)
}

func NormalizeModulePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, ".:")
	if prefix == "" {
		return ""
	}

	rawParts := strings.FieldsFunc(prefix, func(r rune) bool {
		return r == '.' || r == ':'
	})

	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = normalizePart(part)
		if part == "" || part == EmptyPart {
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ":")
}

func CanonicalModulePrefix(prefix string) string {
	return strings.ReplaceAll(NormalizeModulePrefix(prefix), ":", ".")
}

func normalizePart(part string) string {
	part = strings.TrimSpace(part)
	part = strings.Trim(part, ":")
	if part == "" {
		return EmptyPart
	}
	return url.QueryEscape(part)
}
