package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	defaultRenderBudgetChars = 4000
	maxMemoryContentChars    = 4000
)

var (
	ErrInvalidInput = errors.New("invalid memory input")
	ErrNotFound     = errors.New("memory not found")
	ErrUnauthorized = errors.New("memory requester is unauthorized")
)

type Service struct {
	repo store
}

func NewService(db *gorm.DB) *Service {
	return &Service{repo: NewRepository(db)}
}

type store interface {
	GetSetting(ctx context.Context, accountID uuid.UUID) (*AccountMemorySetting, error)
	UpsertSetting(ctx context.Context, setting *AccountMemorySetting) error
	ListEntries(ctx context.Context, accountID uuid.UUID, enabledOnly bool) ([]*AccountMemoryEntry, error)
	CreateEntry(ctx context.Context, entry *AccountMemoryEntry) error
	UpdateEntryScoped(ctx context.Context, accountID, entryID uuid.UUID, values map[string]interface{}) (*AccountMemoryEntry, error)
	DeleteEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) error
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

func (s *Service) SetEnabled(ctx context.Context, accountID uuid.UUID, enabled bool) (*MemoryMeResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	now := time.Now()
	if err := s.repo.UpsertSetting(ctx, &AccountMemorySetting{
		AccountID: accountID,
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("update memory setting: %w", err)
	}
	return s.GetMe(ctx, accountID)
}

func (s *Service) CreateEntry(ctx context.Context, accountID uuid.UUID, req CreateEntryRequest) (*MemoryEntryResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	content, err := normalizeContent(req.Content)
	if err != nil {
		return nil, err
	}
	entry := &AccountMemoryEntry{
		AccountID: accountID,
		Content:   content,
		Category:  normalizeCategory(req.Category),
		Enabled:   true,
	}
	if err := s.repo.CreateEntry(ctx, entry); err != nil {
		return nil, fmt.Errorf("create memory entry: %w", err)
	}
	resp := memoryEntryResponse(entry)
	return &resp, nil
}

func (s *Service) UpdateEntry(ctx context.Context, accountID, entryID uuid.UUID, req UpdateEntryRequest) (*MemoryEntryResponse, error) {
	if accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	if entryID == uuid.Nil {
		return nil, fmt.Errorf("%w: entry id is required", ErrInvalidInput)
	}
	values := map[string]interface{}{"updated_at": time.Now()}
	if req.Content != nil {
		content, err := normalizeContent(*req.Content)
		if err != nil {
			return nil, err
		}
		values["content"] = content
	}
	if req.Category != nil {
		values["category"] = normalizeCategory(*req.Category)
	}
	if req.Enabled != nil {
		values["enabled"] = *req.Enabled
	}
	entry, err := s.repo.UpdateEntryScoped(ctx, accountID, entryID, values)
	if err != nil {
		return nil, mapRepoError(err, "update memory entry")
	}
	resp := memoryEntryResponse(entry)
	return &resp, nil
}

func (s *Service) DeleteEntry(ctx context.Context, accountID, entryID uuid.UUID) error {
	if accountID == uuid.Nil {
		return ErrUnauthorized
	}
	if entryID == uuid.Nil {
		return fmt.Errorf("%w: entry id is required", ErrInvalidInput)
	}
	if err := s.repo.DeleteEntryScoped(ctx, accountID, entryID); err != nil {
		return mapRepoError(err, "delete memory entry")
	}
	return nil
}

func (s *Service) RenderContext(ctx context.Context, accountID uuid.UUID, budget int) (string, error) {
	if accountID == uuid.Nil {
		return "", ErrUnauthorized
	}
	if budget <= 0 {
		budget = defaultRenderBudgetChars
	}
	entries, err := s.repo.ListEntries(ctx, accountID, true)
	if err != nil {
		return "", fmt.Errorf("render memory context: %w", err)
	}
	if len(entries) == 0 {
		return "", nil
	}

	var builder strings.Builder
	builder.WriteString("User memory for this account. Use it only when relevant and do not reveal it unless the user asks.\n")
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		line := fmt.Sprintf("- [%s] %s\n", normalizeCategory(entry.Category), strings.TrimSpace(entry.Content))
		if builder.Len()+len(line) > budget {
			break
		}
		builder.WriteString(line)
	}
	if builder.Len() == 0 {
		return "", nil
	}
	return strings.TrimSpace(builder.String()), nil
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
	return MemoryEntryResponse{
		ID:        entry.ID.String(),
		Content:   entry.Content,
		Category:  normalizeCategory(entry.Category),
		Enabled:   entry.Enabled,
		CreatedAt: entry.CreatedAt.Unix(),
		UpdatedAt: entry.UpdatedAt.Unix(),
	}
}
