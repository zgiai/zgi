package aligner

import (
	"regexp"
	"strings"
)

var (
	// Regex to remove special characters except common punctuation
	specialCharRegex = regexp.MustCompile(`[^\w\s\-\.,]`)
	// Regex to collapse multiple spaces
	spaceRegex = regexp.MustCompile(`\s+`)
)

// Canonicalize standardizes entity names for consistent matching
func Canonicalize(name string) string {
	if name == "" {
		return ""
	}

	// 1. To Lower Case
	s := strings.ToLower(name)

	// 2. Remove special characters (optional, depending on requirements)
	// s = specialCharRegex.ReplaceAllString(s, "")

	// 3. Trim spaces
	s = strings.TrimSpace(s)

	// 4. Collapse multiple spaces
	s = spaceRegex.ReplaceAllString(s, " ")

	return s
}
