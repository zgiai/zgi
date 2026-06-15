package shortlink

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	"gorm.io/gorm"
)

var (
	ErrInvalidToken = errors.New("invalid short link token")
	ErrNotFound     = errors.New("short link not found")
	ErrExpired      = errors.New("short link expired")
)

type Service interface {
	CreateOrGet(ctx context.Context, req CreateOrGetRequest) (*ShortLink, error)
	Resolve(ctx context.Context, token string) (*ShortLink, error)
	SyncKnownTargetExpiresAt(ctx context.Context, now time.Time, limit int) (int64, error)
	CleanupExpired(ctx context.Context, before time.Time, limit int) (int64, error)
	BuildPublicURL(shortToken string) (string, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func NewServiceWithDB(db *gorm.DB) Service {
	return NewService(NewRepository(db))
}

func (s *service) CreateOrGet(ctx context.Context, req CreateOrGetRequest) (*ShortLink, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("short link service is not initialized")
	}
	req.TargetKind = normalizeTargetKind(req.TargetKind)
	req.TargetToken = strings.TrimSpace(req.TargetToken)
	req.TargetPath = strings.TrimSpace(req.TargetPath)
	if req.TargetToken == "" {
		return nil, fmt.Errorf("target_token is required")
	}
	if err := validateTarget(req.TargetKind, req.TargetPath); err != nil {
		return nil, err
	}
	if req.ExpiresAt == nil && targetKindNeedsBusinessExpiration(req.TargetKind) {
		return nil, fmt.Errorf("expires_at is required for %s short links", req.TargetKind)
	}
	pathToken, err := targetTokenFromPath(req.TargetKind, req.TargetPath)
	if err != nil {
		return nil, err
	}
	pathToken, err = url.PathUnescape(pathToken)
	if err != nil {
		return nil, fmt.Errorf("decode target token path segment: %w", err)
	}
	if pathToken != req.TargetToken {
		return nil, fmt.Errorf("target_token must match target_path")
	}

	existing, err := s.repo.GetByTarget(ctx, req.TargetKind, req.TargetToken)
	if err == nil {
		if !sameNullableTime(existing.ExpiresAt, req.ExpiresAt) {
			if err := s.repo.UpdateExpiresAt(ctx, existing.ID, req.ExpiresAt); err != nil {
				return nil, fmt.Errorf("update short link expiration: %w", err)
			}
			existing.ExpiresAt = cloneTime(req.ExpiresAt)
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load short link target: %w", err)
	}

	var createErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		token, err := newToken()
		if err != nil {
			return nil, err
		}
		link := &ShortLink{
			ID:          uuid.NewString(),
			ShortToken:  token,
			TargetKind:  req.TargetKind,
			TargetToken: req.TargetToken,
			TargetPath:  req.TargetPath,
			ExpiresAt:   cloneTime(req.ExpiresAt),
		}
		createErr = s.repo.Create(ctx, link)
		if createErr == nil {
			return link, nil
		}
		if isTargetConflict(createErr) {
			existing, err := s.repo.GetByTarget(ctx, req.TargetKind, req.TargetToken)
			if err != nil {
				return nil, fmt.Errorf("load short link after target conflict: %w", err)
			}
			if !sameNullableTime(existing.ExpiresAt, req.ExpiresAt) {
				if err := s.repo.UpdateExpiresAt(ctx, existing.ID, req.ExpiresAt); err != nil {
					return nil, fmt.Errorf("update short link expiration after target conflict: %w", err)
				}
				existing.ExpiresAt = cloneTime(req.ExpiresAt)
			}
			return existing, nil
		}
		if !isTokenConflict(createErr) {
			return nil, fmt.Errorf("create short link: %w", createErr)
		}
	}
	return nil, fmt.Errorf("create short link after token retries: %w", createErr)
}

func (s *service) Resolve(ctx context.Context, token string) (*ShortLink, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("short link service is not initialized")
	}
	token = normalizeToken(token)
	if !isValidToken(token) {
		return nil, ErrInvalidToken
	}
	link, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("resolve short link: %w", err)
	}
	now := time.Now()
	if link.ExpiresAt == nil && targetKindNeedsBusinessExpiration(link.TargetKind) {
		if err := s.syncLinkExpiresAt(ctx, link, now); err != nil {
			return nil, fmt.Errorf("sync short link expiration: %w", err)
		}
	}
	if isExpired(link, now) {
		return nil, ErrExpired
	}
	return link, nil
}

func (s *service) SyncKnownTargetExpiresAt(ctx context.Context, now time.Time, limit int) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, fmt.Errorf("short link service is not initialized")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	links, err := s.repo.ListMissingExpiresAt(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("list short links missing expiration: %w", err)
	}
	var synced int64
	for i := range links {
		if err := s.syncLinkExpiresAt(ctx, &links[i], now); err != nil {
			return synced, err
		}
		synced++
	}
	return synced, nil
}

func (s *service) CleanupExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, fmt.Errorf("short link service is not initialized")
	}
	if before.IsZero() {
		before = time.Now()
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	if _, err := s.SyncKnownTargetExpiresAt(ctx, before, limit); err != nil {
		return 0, err
	}
	ids, err := s.repo.ListExpiredIDs(ctx, before, limit)
	if err != nil {
		return 0, fmt.Errorf("list expired short links: %w", err)
	}
	deleted, err := s.repo.DeleteByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("delete expired short links: %w", err)
	}
	return deleted, nil
}

func (s *service) BuildPublicURL(shortToken string) (string, error) {
	shortToken = normalizeToken(shortToken)
	if !isValidToken(shortToken) {
		return "", ErrInvalidToken
	}
	base := strings.TrimRight(appconfig.Current().Console.WebURL, "/")
	if base == "" {
		base = strings.TrimRight(appconfig.Current().Email.ConsoleWebURL, "/")
	}
	if base == "" {
		return "", fmt.Errorf("short link public base URL is not configured")
	}
	parsed, err := url.Parse(base)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("short link public base URL is invalid")
	}
	return base + "/" + url.PathEscape(shortToken), nil
}

func (s *service) syncLinkExpiresAt(ctx context.Context, link *ShortLink, now time.Time) error {
	if link == nil || link.ExpiresAt != nil || !targetKindNeedsBusinessExpiration(link.TargetKind) {
		return nil
	}
	expiresAt, found, err := s.repo.LookupKnownTargetExpiresAt(ctx, link.TargetKind, link.TargetToken)
	if err != nil {
		return fmt.Errorf("lookup target expiration: %w", err)
	}
	if !found {
		expiresAt = &now
	}
	if err := s.repo.UpdateExpiresAt(ctx, link.ID, expiresAt); err != nil {
		return fmt.Errorf("update short link expiration: %w", err)
	}
	link.ExpiresAt = cloneTime(expiresAt)
	return nil
}

func targetKindNeedsBusinessExpiration(kind string) bool {
	switch normalizeTargetKind(kind) {
	case TargetKindApprovalForm, TargetKindWorkflowAnnouncement:
		return true
	default:
		return false
	}
}

func isExpired(link *ShortLink, now time.Time) bool {
	if link == nil || link.ExpiresAt == nil {
		return false
	}
	return !link.ExpiresAt.After(now)
}

func sameNullableTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
