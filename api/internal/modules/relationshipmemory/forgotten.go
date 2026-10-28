package relationshipmemory

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	forgottenVersionWindow = 200
	// forgottenOverlap is the share of a forgotten line's terms a text must
	// contain to count as repeating it.
	forgottenOverlap  = 0.6
	forgottenMinTerms = 3
)

// ForgottenLines returns the lines the user's own changes removed from
// memory: what they asked to forget or corrected, and archive entries they
// deleted. Conversation search withholds turns that repeat them (ADR 0014),
// so a forgotten fact does not come back from the conversation history.
// Automatic tidying and archiving are not the user forgetting anything.
func (s *Service) ForgottenLines(ctx context.Context, relationshipID uuid.UUID) ([]string, error) {
	var versions []MemoryFileVersion
	err := s.db.WithContext(ctx).Table("memory_file_versions AS v").Select("v.*").
		Joins("JOIN memory_commits c ON c.id = v.commit_id").
		Where("v.relationship_id = ? AND v.content_purged_at IS NULL AND v.action IN ? AND c.source_kind IN ?",
			relationshipID, []string{VersionUpdate, VersionDelete, VersionClear}, []string{SourceExplicit, SourceManager}).
		Order("v.created_at DESC").Limit(forgottenVersionWindow).Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("list forgotten memory: %w", err)
	}
	seen := map[string]bool{}
	var lines []string
	for _, version := range versions {
		view, err := s.historyVersion(ctx, version)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(view.Before, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*•"))
			if line == "" || seen[line] || RepeatsForgotten(view.After, []string{line}) {
				continue
			}
			seen[line] = true
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// RepeatsForgotten reports whether text contains one of the forgotten lines,
// or most of the words of one.
func RepeatsForgotten(text string, lines []string) bool {
	if strings.TrimSpace(text) == "" || len(lines) == 0 {
		return false
	}
	normalized := normalizeForgotten(text)
	terms := map[string]bool{}
	for _, term := range forgottenTerms(text) {
		terms[term] = true
	}
	for _, line := range lines {
		target := normalizeForgotten(line)
		if utf8.RuneCountInString(target) >= 4 && strings.Contains(normalized, target) {
			return true
		}
		lineTerms := forgottenTerms(line)
		if len(lineTerms) < forgottenMinTerms {
			continue
		}
		shared := 0
		for _, term := range lineTerms {
			if terms[term] {
				shared++
			}
		}
		if float64(shared) >= forgottenOverlap*float64(len(lineTerms)) {
			return true
		}
	}
	return false
}

func normalizeForgotten(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SearchTerms splits text into lowercase words, and runs of CJK text into
// their two-character pieces, for keyword matching across memory stores.
func SearchTerms(value string) []string { return forgottenTerms(value) }

// forgottenTerms splits text into words, and runs of CJK text into their
// two-character pieces.
func forgottenTerms(value string) []string {
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		if utf8.RuneCountInString(term) >= 2 && !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, word := range words {
		runes := []rune(word)
		if len(runes) > 0 && unicode.Is(unicode.Han, runes[0]) {
			for i := 0; i+2 <= len(runes); i++ {
				add(string(runes[i : i+2]))
			}
			continue
		}
		add(word)
	}
	return terms
}
