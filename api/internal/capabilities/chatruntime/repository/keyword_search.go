package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KeywordMatch is one earlier turn found by conversation_search (ADR 0014).
type KeywordMatch struct {
	ConversationID uuid.UUID `gorm:"column:conversation_id"`
	MessageID      uuid.UUID `gorm:"column:message_id"`
	Query          string    `gorm:"column:query"`
	Answer         string    `gorm:"column:answer"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	Score          int       `gorm:"column:score"`
}

const (
	maxSearchKeywords     = 5
	maxSearchKeywordRunes = 40
)

// NormalizeSearchKeywords trims, lowercases, and de-duplicates keywords,
// keeping at most five of at most 40 characters each.
func NormalizeSearchKeywords(keywords []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" || seen[keyword] {
			continue
		}
		if utf8.RuneCountInString(keyword) > maxSearchKeywordRunes {
			keyword = string([]rune(keyword)[:maxSearchKeywordRunes])
		}
		seen[keyword] = true
		out = append(out, keyword)
		if len(out) == maxSearchKeywords {
			break
		}
	}
	return out
}

// SearchMessagesByKeywords finds completed turns of the given conversations
// whose question or answer contains any keyword, ranked by how many keywords
// match and then by recency. conversations is a subquery selecting the
// conversation IDs the caller may search; it is the whole access boundary.
func SearchMessagesByKeywords(ctx context.Context, db *gorm.DB, conversations *gorm.DB, keywords []string, excludeMessageID uuid.UUID, limit int) ([]KeywordMatch, error) {
	keywords = NormalizeSearchKeywords(keywords)
	if db == nil || conversations == nil || len(keywords) == 0 || limit <= 0 {
		return []KeywordMatch{}, nil
	}
	scoreParts := make([]string, 0, len(keywords))
	matchParts := make([]string, 0, len(keywords))
	scoreArgs := make([]any, 0, len(keywords)*2)
	matchArgs := make([]any, 0, len(keywords)*2)
	for _, keyword := range keywords {
		pattern := "%" + escapeLikePattern(keyword) + "%"
		scoreParts = append(scoreParts, `(CASE WHEN LOWER(COALESCE(m.query, '')) LIKE ? ESCAPE '\' OR LOWER(COALESCE(m.answer, '')) LIKE ? ESCAPE '\' THEN 1 ELSE 0 END)`)
		matchParts = append(matchParts, `LOWER(COALESCE(m.query, '')) LIKE ? ESCAPE '\' OR LOWER(COALESCE(m.answer, '')) LIKE ? ESCAPE '\'`)
		scoreArgs = append(scoreArgs, pattern, pattern)
		matchArgs = append(matchArgs, pattern, pattern)
	}
	var matches []KeywordMatch
	query := db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Select(fmt.Sprintf("m.conversation_id, m.id AS message_id, COALESCE(m.query, '') AS query, COALESCE(m.answer, '') AS answer, m.created_at, (%s) AS score", strings.Join(scoreParts, " + ")), scoreArgs...).
		Where("m.conversation_id IN (?)", conversations).
		Where("m.deleted_at IS NULL AND m.status = ?", "completed").
		Where("("+strings.Join(matchParts, " OR ")+")", matchArgs...)
	if excludeMessageID != uuid.Nil {
		query = query.Where("m.id <> ?", excludeMessageID)
	}
	if err := query.Order("score DESC, m.created_at DESC").Limit(limit).Scan(&matches).Error; err != nil {
		return nil, fmt.Errorf("search conversation messages: %w", err)
	}
	return matches, nil
}
