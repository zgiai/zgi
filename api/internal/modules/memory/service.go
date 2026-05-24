package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultRenderBudgetChars   = 4000
	maxMemoryContentChars      = 4000
	maxMemoryEntriesPerAccount = 200
	maxTemporaryRenderEntries  = 20
	memorySimilarityCutoff     = 0.92
	memoryStatusActive         = "active"
	memoryStatusExpired        = "expired"
)

const memoryRenderPolicy = `User memory is enabled for this account.
When to use user-memory:
- Use it when the user asks to remember, forget, update, or rely on memory.
- MUST call user-memory for stable preferences, personal info, habits, address forms, or standing instructions.
- Names, preferences, habits, and address forms are memory-worthy without "remember".
- Resolve relative dates before saving temporary memory.
- Read memory if duplicates/conflicts may exist; update/merge instead of duplicating.
- For ordinary non-sensitive memory-worthy information, do not ask whether to save it; save quietly.
- ask for confirmation only when new info conflicts with saved memory, is ambiguous, or sensitive.
- do not say memory was remembered, saved, updated, or deleted unless the tool succeeds this turn.
- After writing, keep answering naturally; mention memory only if the user explicitly asked.
- Skip secrets, sensitive data, one-off chat, and tenant/workspace facts unless explicit.
`

var (
	ErrInvalidInput = errors.New("invalid memory input")
	ErrNotFound     = errors.New("memory not found")
	ErrUnauthorized = errors.New("memory requester is unauthorized")
	ErrDisabled     = errors.New("memory is disabled")
)

type Service struct {
	repo store
}

func NewService(db *gorm.DB) *Service {
	return &Service{repo: NewRepository(db)}
}

type store interface {
	WithTransaction(ctx context.Context, fn func(store) error) error
	GetSetting(ctx context.Context, accountID uuid.UUID) (*AccountMemorySetting, error)
	UpsertSetting(ctx context.Context, setting *AccountMemorySetting) error
	LockAccount(ctx context.Context, accountID uuid.UUID) error
	ListEntries(ctx context.Context, accountID uuid.UUID, enabledOnly bool) ([]*AccountMemoryEntry, error)
	CreateEntry(ctx context.Context, entry *AccountMemoryEntry) error
	GetEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) (*AccountMemoryEntry, error)
	UpdateEntryScoped(ctx context.Context, accountID, entryID uuid.UUID, values map[string]interface{}) (*AccountMemoryEntry, error)
	DeleteEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) error
	CreateEvent(ctx context.Context, event *AccountMemoryEvent) error
}

type MutationMetadata struct {
	ActorType            string
	Source               string
	SourceConversationID *uuid.UUID
	SourceMessageID      *uuid.UUID
}

func (s *Service) GetMe(ctx context.Context, accountID uuid.UUID) (*MemoryMeResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	setting, err := s.repo.GetSetting(ctx, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			setting = &AccountMemorySetting{AccountID: accountID, Enabled: false}
		} else {
			return nil, fmt.Errorf("get memory setting: %w", err)
		}
	}
	entries, err := s.repo.ListEntries(ctx, accountID, false)
	if err != nil {
		return nil, fmt.Errorf("list memory entries: %w", err)
	}
	return memoryMeResponse(setting, entries), nil
}

func (s *Service) GetModelState(ctx context.Context, accountID uuid.UUID) (*MemoryMeResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	setting, err := s.repo.GetSetting(ctx, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			setting = &AccountMemorySetting{AccountID: accountID, Enabled: false}
		} else {
			return nil, fmt.Errorf("get memory setting: %w", err)
		}
	}
	entries, err := s.repo.ListEntries(ctx, accountID, true)
	if err != nil {
		return nil, fmt.Errorf("list enabled memory entries: %w", err)
	}
	return memoryMeResponse(setting, entries), nil
}

func (s *Service) IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error) {
	if accountID == uuid.Nil {
		return false, ErrUnauthorized
	}
	setting, err := s.repo.GetSetting(ctx, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("get memory setting: %w", err)
	}
	return setting.Enabled, nil
}

func (s *Service) SetEnabled(ctx context.Context, accountID uuid.UUID, enabled bool) (*MemoryMeResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}

	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		before, err := tx.GetSetting(ctx, accountID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get memory setting: %w", err)
		}
		now := time.Now()
		setting := &AccountMemorySetting{
			AccountID: accountID,
			Enabled:   enabled,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.UpsertSetting(ctx, setting); err != nil {
			return fmt.Errorf("update memory setting: %w", err)
		}
		return recordEvent(ctx, tx, accountID, nil, settingAction(enabled), defaultMutationMetadata(), settingSnapshot(before), settingSnapshot(setting))
	}); err != nil {
		return nil, err
	}
	return s.GetMe(ctx, accountID)
}

func (s *Service) CreateEntry(ctx context.Context, accountID uuid.UUID, req CreateEntryRequest) (*MemoryEntryResponse, error) {
	return s.CreateEntryWithMetadata(ctx, accountID, req, defaultMutationMetadata())
}

func (s *Service) CreateEntryWithMetadata(ctx context.Context, accountID uuid.UUID, req CreateEntryRequest, meta MutationMetadata) (*MemoryEntryResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	content, err := normalizeContent(req.Content)
	if err != nil {
		return nil, err
	}
	category := normalizeCategory(req.Category)
	memoryType, expiresAt, err := resolveCreateTiming(req.MemoryType, req.ExpiresAt)
	if err != nil {
		return nil, err
	}
	var response *MemoryEntryResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		if err := tx.LockAccount(ctx, accountID); err != nil {
			return fmt.Errorf("lock memory account: %w", err)
		}
		existingEntries, err := tx.ListEntries(ctx, accountID, false)
		if err != nil {
			return fmt.Errorf("list memory entries: %w", err)
		}
		if existing := findMergeCandidate(content, category, memoryType, existingEntries); existing != nil {
			before := *existing
			values := map[string]interface{}{
				"content":     content,
				"category":    chooseMergedCategory(existing.Category, category),
				"memory_type": memoryType,
				"expires_at":  expiresAt,
				"enabled":     true,
				"updated_at":  time.Now(),
			}
			entry, err := tx.UpdateEntryScoped(ctx, accountID, existing.ID, values)
			if err != nil {
				return mapRepoError(err, "merge memory entry")
			}
			if err := recordEntryEvent(ctx, tx, accountID, &entry.ID, EventActionUpdate, meta, &before, entry); err != nil {
				return err
			}
			resp := memoryEntryResponse(entry)
			response = &resp
			return nil
		}
		if len(existingEntries) >= maxMemoryEntriesPerAccount {
			return fmt.Errorf("%w: memory entry limit reached", ErrInvalidInput)
		}
		entry := &AccountMemoryEntry{
			AccountID:  accountID,
			Content:    content,
			Category:   category,
			MemoryType: memoryType,
			ExpiresAt:  expiresAt,
			Enabled:    true,
		}
		if err := tx.CreateEntry(ctx, entry); err != nil {
			return fmt.Errorf("create memory entry: %w", err)
		}
		if err := recordEntryEvent(ctx, tx, accountID, &entry.ID, EventActionCreate, meta, nil, entry); err != nil {
			return err
		}
		resp := memoryEntryResponse(entry)
		response = &resp
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) UpdateEntry(ctx context.Context, accountID, entryID uuid.UUID, req UpdateEntryRequest) (*MemoryEntryResponse, error) {
	return s.UpdateEntryWithMetadata(ctx, accountID, entryID, req, defaultMutationMetadata())
}

func (s *Service) UpdateEntryWithMetadata(ctx context.Context, accountID, entryID uuid.UUID, req UpdateEntryRequest, meta MutationMetadata) (*MemoryEntryResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	if entryID == uuid.Nil {
		return nil, fmt.Errorf("%w: entry id is required", ErrInvalidInput)
	}
	var response *MemoryEntryResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		if err := tx.LockAccount(ctx, accountID); err != nil {
			return fmt.Errorf("lock memory account: %w", err)
		}
		lockedBefore, err := tx.GetEntryScoped(ctx, accountID, entryID)
		if err != nil {
			return mapRepoError(err, "get memory entry")
		}
		values := map[string]interface{}{"updated_at": time.Now()}
		if req.Content != nil {
			content, err := normalizeContent(*req.Content)
			if err != nil {
				return err
			}
			values["content"] = content
		}
		if req.Category != nil {
			values["category"] = normalizeCategory(*req.Category)
		}
		memoryType, expiresAt, err := resolveUpdateTiming(lockedBefore, req.MemoryType, req.ExpiresAt)
		if err != nil {
			return err
		}
		values["memory_type"] = memoryType
		values["expires_at"] = expiresAt
		if req.Enabled != nil {
			values["enabled"] = *req.Enabled
		}
		entry, err := tx.UpdateEntryScoped(ctx, accountID, entryID, values)
		if err != nil {
			return mapRepoError(err, "update memory entry")
		}
		if err := recordEntryEvent(ctx, tx, accountID, &entry.ID, updateAction(lockedBefore, entry), meta, lockedBefore, entry); err != nil {
			return err
		}
		resp := memoryEntryResponse(entry)
		response = &resp
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) DeleteEntry(ctx context.Context, accountID, entryID uuid.UUID) error {
	return s.DeleteEntryWithMetadata(ctx, accountID, entryID, defaultMutationMetadata())
}

func (s *Service) DeleteEntryWithMetadata(ctx context.Context, accountID, entryID uuid.UUID, meta MutationMetadata) error {
	if accountID == uuid.Nil {
		return ErrUnauthorized
	}
	if entryID == uuid.Nil {
		return fmt.Errorf("%w: entry id is required", ErrInvalidInput)
	}
	return s.repo.WithTransaction(ctx, func(tx store) error {
		if err := tx.LockAccount(ctx, accountID); err != nil {
			return fmt.Errorf("lock memory account: %w", err)
		}
		before, err := tx.GetEntryScoped(ctx, accountID, entryID)
		if err != nil {
			return mapRepoError(err, "get memory entry")
		}
		if err := tx.DeleteEntryScoped(ctx, accountID, entryID); err != nil {
			return mapRepoError(err, "delete memory entry")
		}
		return recordEntryEvent(ctx, tx, accountID, &entryID, EventActionDelete, meta, before, nil)
	})
}

func (s *Service) RenderContext(ctx context.Context, accountID uuid.UUID, budget int) (string, error) {
	if accountID == uuid.Nil {
		return "", ErrUnauthorized
	}
	if budget <= 0 {
		budget = defaultRenderBudgetChars
	}
	enabled, err := s.IsEnabled(ctx, accountID)
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", nil
	}
	entries, err := s.repo.ListEntries(ctx, accountID, true)
	if err != nil {
		return "", fmt.Errorf("render memory context: %w", err)
	}
	longTerm, temporary := splitRenderableEntries(entries, time.Now())
	return renderMemoryEntries(longTerm, temporary, budget), nil
}

func (s *Service) ListTemporaryEntries(ctx context.Context, accountID uuid.UUID, status string, limit int) ([]MemoryEntryResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	status = normalizeTemporaryStatus(status)
	entries, err := s.repo.ListEntries(ctx, accountID, true)
	if err != nil {
		return nil, fmt.Errorf("list temporary memories: %w", err)
	}
	now := time.Now()
	filtered := make([]*AccountMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || normalizeMemoryType(entry.MemoryType) != MemoryTypeTemporary {
			continue
		}
		entryStatus := memoryStatus(entry, now)
		if status != "all" && entryStatus != status {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := filtered[i]
		right := filtered[j]
		if status == memoryStatusExpired {
			if left.ExpiresAt != nil && right.ExpiresAt != nil && !left.ExpiresAt.Equal(*right.ExpiresAt) {
				return left.ExpiresAt.After(*right.ExpiresAt)
			}
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		if left.ExpiresAt != nil && right.ExpiresAt != nil && !left.ExpiresAt.Equal(*right.ExpiresAt) {
			return left.ExpiresAt.Before(*right.ExpiresAt)
		}
		return left.UpdatedAt.After(right.UpdatedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	responses := make([]MemoryEntryResponse, 0, len(filtered))
	for _, entry := range filtered {
		responses = append(responses, memoryEntryResponse(entry))
	}
	return responses, nil
}

func renderMemoryEntries(longTerm []*AccountMemoryEntry, temporary []*AccountMemoryEntry, budget int) string {
	var builder strings.Builder
	if len(memoryRenderPolicy) > budget {
		return ""
	}
	builder.WriteString(memoryRenderPolicy)

	if len(longTerm) > 0 {
		if writeSectionHeader(&builder, "Long-term memory:\n", budget) {
			renderCategorizedEntries(&builder, longTerm, budget)
		}
	}

	if len(temporary) > 0 {
		if writeSectionHeader(&builder, "Temporary memory:\n", budget) {
			for i, entry := range temporary {
				if i >= maxTemporaryRenderEntries {
					break
				}
				line := temporaryMemoryLine(entry)
				if builder.Len()+len(line) > budget {
					return strings.TrimSpace(builder.String())
				}
				builder.WriteString(line)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func ResolveToolAccountID(userID string) (uuid.UUID, error) {
	accountID, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return uuid.Nil, ErrUnauthorized
	}
	return accountID, nil
}

func normalizeContent(raw string) (string, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return "", fmt.Errorf("%w: content is required", ErrInvalidInput)
	}
	if len([]rune(content)) > maxMemoryContentChars {
		return "", fmt.Errorf("%w: content is too long", ErrInvalidInput)
	}
	return content, nil
}

func normalizeCategory(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CategoryPreference:
		return CategoryPreference
	case CategoryProfile:
		return CategoryProfile
	case CategoryInstruction:
		return CategoryInstruction
	case CategoryFact:
		return CategoryFact
	default:
		return CategoryOther
	}
}

func normalizeMemoryType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case MemoryTypeTemporary:
		return MemoryTypeTemporary
	default:
		return MemoryTypeLongTerm
	}
}

func resolveCreateTiming(rawType string, rawExpiresAt string) (string, *time.Time, error) {
	expiresAt, err := parseOptionalExpiresAt(rawExpiresAt)
	if err != nil {
		return "", nil, err
	}
	memoryType := normalizeMemoryType(rawType)
	if strings.TrimSpace(rawType) == "" && expiresAt != nil {
		memoryType = MemoryTypeTemporary
	}
	if memoryType == MemoryTypeLongTerm {
		return memoryType, nil, nil
	}
	if expiresAt == nil {
		return "", nil, fmt.Errorf("%w: temporary memory requires expires_at", ErrInvalidInput)
	}
	return memoryType, expiresAt, nil
}

func resolveUpdateTiming(entry *AccountMemoryEntry, rawType *string, rawExpiresAt *string) (string, *time.Time, error) {
	memoryType := MemoryTypeLongTerm
	var expiresAt *time.Time
	if entry != nil {
		memoryType = normalizeMemoryType(entry.MemoryType)
		expiresAt = entry.ExpiresAt
	}
	if rawType != nil {
		memoryType = normalizeMemoryType(*rawType)
	}
	if rawExpiresAt != nil {
		parsed, err := parseOptionalExpiresAt(*rawExpiresAt)
		if err != nil {
			return "", nil, err
		}
		expiresAt = parsed
		if rawType == nil && parsed != nil {
			memoryType = MemoryTypeTemporary
		}
	}
	if memoryType == MemoryTypeLongTerm {
		return memoryType, nil, nil
	}
	if expiresAt == nil {
		return "", nil, fmt.Errorf("%w: temporary memory requires expires_at", ErrInvalidInput)
	}
	return memoryType, expiresAt, nil
}

func parseOptionalExpiresAt(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	if unix, err := parseUnixTimestamp(value); err == nil {
		t := time.Unix(unix, 0).UTC()
		return &t, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			t := parsed.UTC()
			return &t, nil
		}
	}
	return nil, fmt.Errorf("%w: expires_at must be RFC3339 or unix timestamp", ErrInvalidInput)
}

func parseUnixTimestamp(value string) (int64, error) {
	unix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	if unix <= 0 {
		return 0, fmt.Errorf("timestamp must be positive")
	}
	if unix > 1e12 {
		unix = unix / 1000
	}
	return unix, nil
}

type categoryPolicy struct {
	category   string
	label      string
	maxEntries int
}

func memoryCategoryPolicies() []categoryPolicy {
	return []categoryPolicy{
		{category: CategoryInstruction, label: "Standing instructions", maxEntries: 12},
		{category: CategoryPreference, label: "Preferences", maxEntries: 20},
		{category: CategoryProfile, label: "Profile", maxEntries: 20},
		{category: CategoryFact, label: "Stable facts", maxEntries: 30},
		{category: CategoryOther, label: "Other memories", maxEntries: 10},
	}
}

func splitRenderableEntries(entries []*AccountMemoryEntry, now time.Time) ([]*AccountMemoryEntry, []*AccountMemoryEntry) {
	longTerm := make([]*AccountMemoryEntry, 0, len(entries))
	temporary := make([]*AccountMemoryEntry, 0)
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if normalizeMemoryType(entry.MemoryType) == MemoryTypeTemporary {
			if isTemporaryMemoryActive(entry, now) {
				temporary = append(temporary, entry)
			}
			continue
		}
		longTerm = append(longTerm, entry)
	}
	sort.SliceStable(temporary, func(i, j int) bool {
		left := temporary[i]
		right := temporary[j]
		if left.ExpiresAt == nil && right.ExpiresAt != nil {
			return false
		}
		if left.ExpiresAt != nil && right.ExpiresAt == nil {
			return true
		}
		if left.ExpiresAt != nil && right.ExpiresAt != nil && !left.ExpiresAt.Equal(*right.ExpiresAt) {
			return left.ExpiresAt.Before(*right.ExpiresAt)
		}
		return left.UpdatedAt.After(right.UpdatedAt)
	})
	return longTerm, temporary
}

func renderCategorizedEntries(builder *strings.Builder, entries []*AccountMemoryEntry, budget int) bool {
	wroteEntry := false
	grouped := groupEntriesByCategory(entries)
	for _, policy := range memoryCategoryPolicies() {
		group := grouped[policy.category]
		if len(group) == 0 {
			continue
		}
		section := policy.label + ":\n"
		if builder.Len()+len(section) > budget {
			break
		}
		builder.WriteString(section)
		for i, entry := range group {
			if policy.maxEntries > 0 && i >= policy.maxEntries {
				break
			}
			line := fmt.Sprintf("- %s\n", strings.TrimSpace(entry.Content))
			if builder.Len()+len(line) > budget {
				return wroteEntry
			}
			builder.WriteString(line)
			wroteEntry = true
		}
	}
	return wroteEntry
}

func writeSectionHeader(builder *strings.Builder, header string, budget int) bool {
	if builder.Len()+len(header) > budget {
		return false
	}
	builder.WriteString(header)
	return true
}

func temporaryMemoryLine(entry *AccountMemoryEntry) string {
	content := strings.TrimSpace(entry.Content)
	if entry.ExpiresAt == nil {
		return fmt.Sprintf("- %s\n", content)
	}
	return fmt.Sprintf("- %s (valid until %s)\n", content, entry.ExpiresAt.UTC().Format(time.RFC3339))
}

func isTemporaryMemoryActive(entry *AccountMemoryEntry, now time.Time) bool {
	return normalizeMemoryType(entry.MemoryType) == MemoryTypeTemporary &&
		entry.ExpiresAt != nil &&
		entry.ExpiresAt.After(now)
}

func memoryStatus(entry *AccountMemoryEntry, now time.Time) string {
	if entry == nil || normalizeMemoryType(entry.MemoryType) != MemoryTypeTemporary {
		return memoryStatusActive
	}
	if isTemporaryMemoryActive(entry, now) {
		return memoryStatusActive
	}
	return memoryStatusExpired
}

func normalizeTemporaryStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case memoryStatusExpired:
		return memoryStatusExpired
	case "all":
		return "all"
	default:
		return memoryStatusActive
	}
}

func groupEntriesByCategory(entries []*AccountMemoryEntry) map[string][]*AccountMemoryEntry {
	grouped := map[string][]*AccountMemoryEntry{}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		category := normalizeCategory(entry.Category)
		grouped[category] = append(grouped[category], entry)
	}
	for category := range grouped {
		sort.SliceStable(grouped[category], func(i, j int) bool {
			return grouped[category][i].UpdatedAt.After(grouped[category][j].UpdatedAt)
		})
	}
	return grouped
}

func chooseMergedCategory(existing string, incoming string) string {
	existing = normalizeCategory(existing)
	incoming = normalizeCategory(incoming)
	if existing == CategoryOther {
		return incoming
	}
	if existing == CategoryFact && incoming != CategoryOther {
		return incoming
	}
	return existing
}

func findMergeCandidate(content string, category string, memoryType string, entries []*AccountMemoryEntry) *AccountMemoryEntry {
	contentKey := canonicalMemoryText(content)
	if contentKey == "" {
		return nil
	}
	var best *AccountMemoryEntry
	bestScore := 0.0
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if normalizeMemoryType(entry.MemoryType) != normalizeMemoryType(memoryType) {
			continue
		}
		entryCategory := normalizeCategory(entry.Category)
		if entryCategory != category && entryCategory != CategoryOther && category != CategoryOther {
			continue
		}
		entryKey := canonicalMemoryText(entry.Content)
		if entryKey == "" {
			continue
		}
		if entryKey == contentKey {
			return entry
		}
		score := memorySimilarity(contentKey, entryKey)
		if score > bestScore {
			bestScore = score
			best = entry
		}
	}
	if bestScore >= memorySimilarityCutoff {
		return best
	}
	return nil
}

func canonicalMemoryText(raw string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(raw) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func memorySimilarity(a string, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter, longer := len([]rune(a)), len([]rune(b))
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return float64(shorter) / float64(longer)
	}
	aGrams := bigrams(a)
	bGrams := bigrams(b)
	if len(aGrams) == 0 || len(bGrams) == 0 {
		return 0
	}
	overlap := 0
	for gram := range aGrams {
		if _, ok := bGrams[gram]; ok {
			overlap++
		}
	}
	return float64(2*overlap) / float64(len(aGrams)+len(bGrams))
}

func bigrams(value string) map[string]struct{} {
	runes := []rune(value)
	grams := map[string]struct{}{}
	if len(runes) == 1 {
		grams[string(runes)] = struct{}{}
		return grams
	}
	for i := 0; i < len(runes)-1; i++ {
		grams[string(runes[i:i+2])] = struct{}{}
	}
	return grams
}

func defaultMutationMetadata() MutationMetadata {
	return MutationMetadata{
		ActorType: EventActorUser,
		Source:    EventSourceAPI,
	}
}

func normalizeMutationMetadata(meta MutationMetadata) MutationMetadata {
	if meta.ActorType == "" {
		meta.ActorType = EventActorUser
	}
	if meta.Source == "" {
		meta.Source = EventSourceAPI
	}
	return meta
}

func settingAction(enabled bool) string {
	if enabled {
		return EventActionEnable
	}
	return EventActionDisable
}

func updateAction(before *AccountMemoryEntry, after *AccountMemoryEntry) string {
	if before != nil && after != nil && before.Enabled != after.Enabled {
		if after.Enabled {
			return EventActionEnable
		}
		return EventActionDisable
	}
	return EventActionUpdate
}

func recordEntryEvent(ctx context.Context, repo store, accountID uuid.UUID, entryID *uuid.UUID, action string, meta MutationMetadata, before *AccountMemoryEntry, after *AccountMemoryEntry) error {
	return recordEvent(ctx, repo, accountID, entryID, action, meta, entrySnapshot(before), entrySnapshot(after))
}

func recordEvent(ctx context.Context, repo store, accountID uuid.UUID, entryID *uuid.UUID, action string, meta MutationMetadata, before datatypes.JSON, after datatypes.JSON) error {
	meta = normalizeMutationMetadata(meta)
	event := &AccountMemoryEvent{
		AccountID:            accountID,
		EntryID:              entryID,
		Action:               action,
		ActorType:            meta.ActorType,
		Source:               meta.Source,
		SourceConversationID: meta.SourceConversationID,
		SourceMessageID:      meta.SourceMessageID,
		BeforeSnapshot:       before,
		AfterSnapshot:        after,
	}
	if err := repo.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("record memory event: %w", err)
	}
	return nil
}

func entrySnapshot(entry *AccountMemoryEntry) datatypes.JSON {
	if entry == nil {
		return datatypes.JSON([]byte("null"))
	}
	return mustJSON(map[string]interface{}{
		"id":          entry.ID.String(),
		"account_id":  entry.AccountID.String(),
		"content":     entry.Content,
		"category":    normalizeCategory(entry.Category),
		"memory_type": normalizeMemoryType(entry.MemoryType),
		"expires_at":  entryExpiresAtUnix(entry),
		"status":      memoryStatus(entry, time.Now()),
		"enabled":     entry.Enabled,
		"created_at":  entry.CreatedAt.Unix(),
		"updated_at":  entry.UpdatedAt.Unix(),
	})
}

func settingSnapshot(setting *AccountMemorySetting) datatypes.JSON {
	if setting == nil {
		return datatypes.JSON([]byte("null"))
	}
	return mustJSON(map[string]interface{}{
		"account_id": setting.AccountID.String(),
		"enabled":    setting.Enabled,
		"created_at": setting.CreatedAt.Unix(),
		"updated_at": setting.UpdatedAt.Unix(),
	})
}

func mustJSON(value interface{}) datatypes.JSON {
	data, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return datatypes.JSON(data)
}

func mapRepoError(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", message, err)
}

func memoryMeResponse(setting *AccountMemorySetting, entries []*AccountMemoryEntry) *MemoryMeResponse {
	resp := &MemoryMeResponse{
		Enabled: false,
		Entries: make([]MemoryEntryResponse, 0, len(entries)),
	}
	if setting != nil {
		resp.Enabled = setting.Enabled
		resp.UpdatedAt = setting.UpdatedAt.Unix()
	}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		resp.Entries = append(resp.Entries, memoryEntryResponse(entry))
		if entry.UpdatedAt.Unix() > resp.UpdatedAt {
			resp.UpdatedAt = entry.UpdatedAt.Unix()
		}
	}
	return resp
}

func memoryEntryResponse(entry *AccountMemoryEntry) MemoryEntryResponse {
	expiresAt := entryExpiresAtUnix(entry)
	return MemoryEntryResponse{
		ID:         entry.ID.String(),
		Content:    entry.Content,
		Category:   normalizeCategory(entry.Category),
		MemoryType: normalizeMemoryType(entry.MemoryType),
		ExpiresAt:  expiresAt,
		Status:     memoryStatus(entry, time.Now()),
		Enabled:    entry.Enabled,
		CreatedAt:  entry.CreatedAt.Unix(),
		UpdatedAt:  entry.UpdatedAt.Unix(),
	}
}

func entryExpiresAtUnix(entry *AccountMemoryEntry) *int64 {
	if entry == nil || entry.ExpiresAt == nil {
		return nil
	}
	value := entry.ExpiresAt.Unix()
	return &value
}
