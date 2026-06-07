package announcement

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
	"gorm.io/gorm"
)

type Service struct {
	db               *gorm.DB
	shortLinkService shortlinkcap.Service
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func NewServiceWithShortLinkService(db *gorm.DB, shortLinkService shortlinkcap.Service) *Service {
	return &Service{db: db, shortLinkService: shortLinkService}
}

func (s *Service) CreateOrGetRuntimeAnnouncement(ctx context.Context, params CreateRuntimeAnnouncementParams) (*RuntimeAnnouncement, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("announcement service is not initialized")
	}
	if err := validateRuntimeParams(params); err != nil {
		return nil, err
	}

	existing, err := s.loadRuntimeAnnouncement(ctx, params.TenantID, params.WorkflowRunID, params.NodeID)
	if err == nil {
		return s.runtimeAnnouncementPayload(ctx, existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load announcement: %w", err)
	}

	announcement, err := buildRuntimeAnnouncement(params)
	if err != nil {
		return nil, err
	}
	created, err := s.createRuntimeAnnouncementWithTokenRetry(ctx, announcement)
	if err != nil {
		return nil, err
	}
	return s.runtimeAnnouncementPayload(ctx, created)
}

func (s *Service) GetByToken(ctx context.Context, token string) (*AnnouncementPayload, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("announcement service is not initialized")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrAnnouncementNotFound
	}

	var announcement Announcement
	if err := s.db.WithContext(ctx).First(&announcement, "access_token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, fmt.Errorf("load announcement: %w", err)
	}
	if time.Now().After(announcement.ExpirationTime) {
		return nil, ErrAnnouncementExpired
	}
	payload := announcementPayload(&announcement)
	return &payload, nil
}

func (s *Service) createRuntimeAnnouncementWithTokenRetry(ctx context.Context, announcement *Announcement) (*Announcement, error) {
	var createErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		if attempt > 0 {
			token, err := newAnnouncementToken()
			if err != nil {
				return nil, err
			}
			announcement.AccessToken = token
		}
		createErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(announcement).Error; err != nil {
				return err
			}
			if err := createAnnouncementShortLink(ctx, tx, announcement); err != nil {
				return err
			}
			return nil
		})
		if createErr == nil {
			return announcement, nil
		}
		if isAnnouncementRunNodeConflict(createErr) {
			existing, err := s.loadRuntimeAnnouncement(ctx, announcement.TenantID, announcement.WorkflowRunID, announcement.NodeID)
			if err != nil {
				return nil, fmt.Errorf("load announcement after run/node conflict: %w", err)
			}
			return existing, nil
		}
		if !isAnnouncementTokenConflict(createErr) {
			return nil, fmt.Errorf("create announcement: %w", createErr)
		}
	}
	return nil, fmt.Errorf("create announcement after token retries: %w", createErr)
}

func (s *Service) loadRuntimeAnnouncement(ctx context.Context, tenantID, workflowRunID, nodeID string) (*Announcement, error) {
	var existing Announcement
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND workflow_run_id = ? AND node_id = ?", tenantID, workflowRunID, nodeID).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *Service) CleanupExpiredAnnouncements(ctx context.Context, before time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("announcement service is not initialized")
	}
	if before.IsZero() {
		before = time.Now()
	}
	result := s.db.WithContext(ctx).Where("expiration_time <= ?", before).Delete(&Announcement{})
	if result.Error != nil {
		return 0, fmt.Errorf("cleanup expired announcements: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (s *Service) runtimeAnnouncementPayload(ctx context.Context, announcement *Announcement) (*RuntimeAnnouncement, error) {
	payload := announcementPayload(announcement)
	shortLink, err := s.announcementShortLink(ctx, announcement)
	if err != nil {
		return nil, err
	}
	service := s.shortLinkCapability()
	payload.Token = shortLink.ShortToken
	shortURL, err := service.BuildPublicURL(shortLink.ShortToken)
	if err != nil {
		return nil, err
	}
	payload.URL = shortURL
	return &RuntimeAnnouncement{
		Announcement: announcement,
		Payload:      payload,
	}, nil
}

func buildRuntimeAnnouncement(params CreateRuntimeAnnouncementParams) (*Announcement, error) {
	token, err := newAnnouncementToken()
	if err != nil {
		return nil, err
	}
	return &Announcement{
		ID:              uuid.NewString(),
		TenantID:        params.TenantID,
		AppID:           params.AppID,
		WorkflowRunID:   params.WorkflowRunID,
		NodeID:          params.NodeID,
		NodeTitle:       params.NodeTitle,
		Content:         params.Config.Content,
		RenderedContent: params.Rendered,
		AccessToken:     token,
		ExpirationTime:  expirationTime(params.Config.Timeout),
	}, nil
}

func announcementPayload(announcement *Announcement) AnnouncementPayload {
	if announcement == nil {
		return AnnouncementPayload{}
	}
	return AnnouncementPayload{
		ID:           announcement.ID,
		Token:        announcement.AccessToken,
		AccessToken:  announcement.AccessToken,
		NodeID:       announcement.NodeID,
		Title:        announcement.NodeTitle,
		NodeTitle:    announcement.NodeTitle,
		Content:      announcement.RenderedContent,
		ExpirationAt: announcement.ExpirationTime.Unix(),
		Expired:      time.Now().After(announcement.ExpirationTime),
	}
}

func validateRuntimeParams(params CreateRuntimeAnnouncementParams) error {
	if strings.TrimSpace(params.TenantID) == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(params.AppID) == "" {
		return fmt.Errorf("app_id is required")
	}
	if strings.TrimSpace(params.WorkflowRunID) == "" {
		return fmt.Errorf("workflow_run_id is required")
	}
	if strings.TrimSpace(params.NodeID) == "" {
		return fmt.Errorf("node_id is required")
	}
	if len([]rune(strings.TrimSpace(params.NodeTitle))) > MaxTitleLength {
		return fmt.Errorf("announcement title cannot exceed %d characters", MaxTitleLength)
	}
	return ValidateConfig(params.Config)
}

func ValidateConfig(config NodeConfig) error {
	title := strings.TrimSpace(config.Title)
	if title == "" {
		return fmt.Errorf("announcement title is required")
	}
	if len([]rune(title)) > MaxTitleLength {
		return fmt.Errorf("announcement title cannot exceed %d characters", MaxTitleLength)
	}
	if strings.TrimSpace(config.Content) == "" {
		return fmt.Errorf("announcement content is required")
	}
	duration := config.Timeout.Duration
	if duration <= 0 {
		duration = defaultTimeoutDuration
	}
	unit := strings.TrimSpace(config.Timeout.Unit)
	if unit == "" {
		unit = defaultTimeoutUnit
	}
	switch unit {
	case "hour", "hours":
		if duration > 168 {
			return fmt.Errorf("announcement timeout cannot exceed 168 hours")
		}
	case "day", "days":
		if duration > 7 {
			return fmt.Errorf("announcement timeout cannot exceed 7 days")
		}
	default:
		return fmt.Errorf("unsupported announcement timeout unit: %s", unit)
	}
	return nil
}

func expirationTime(timeout TimeoutConfig) time.Time {
	duration := timeout.Duration
	if duration <= 0 {
		duration = defaultTimeoutDuration
	}
	unit := strings.TrimSpace(timeout.Unit)
	if unit == "" {
		unit = defaultTimeoutUnit
	}
	now := time.Now()
	switch unit {
	case "day", "days":
		return now.Add(time.Duration(duration) * 24 * time.Hour)
	default:
		return now.Add(time.Duration(duration) * time.Hour)
	}
}

func (s *Service) announcementShortLink(ctx context.Context, announcement *Announcement) (*shortlinkcap.ShortLink, error) {
	if announcement == nil {
		return nil, fmt.Errorf("announcement is required")
	}
	accessToken := strings.TrimSpace(announcement.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("announcement access token is required")
	}
	service := s.shortLinkCapability()
	return service.CreateOrGet(ctx, shortlinkcap.CreateOrGetRequest{
		TargetKind:  shortlinkcap.TargetKindWorkflowAnnouncement,
		TargetToken: accessToken,
		TargetPath:  announcementTargetPath(accessToken),
		ExpiresAt:   &announcement.ExpirationTime,
	})
}

func createAnnouncementShortLink(ctx context.Context, db *gorm.DB, announcement *Announcement) error {
	if announcement == nil {
		return fmt.Errorf("announcement is required")
	}
	accessToken := strings.TrimSpace(announcement.AccessToken)
	if accessToken == "" {
		return fmt.Errorf("announcement access token is required")
	}
	shortLinkService := shortlinkcap.NewServiceWithDB(db)
	if _, err := shortLinkService.CreateOrGet(ctx, shortlinkcap.CreateOrGetRequest{
		TargetKind:  shortlinkcap.TargetKindWorkflowAnnouncement,
		TargetToken: accessToken,
		TargetPath:  announcementTargetPath(accessToken),
		ExpiresAt:   &announcement.ExpirationTime,
	}); err != nil {
		return fmt.Errorf("create announcement short link: %w", err)
	}
	return nil
}

func (s *Service) shortLinkCapability() shortlinkcap.Service {
	if s != nil && s.shortLinkService != nil {
		return s.shortLinkService
	}
	if s == nil {
		return shortlinkcap.NewService(nil)
	}
	return shortlinkcap.NewServiceWithDB(s.db)
}

func announcementTargetPath(token string) string {
	return announcementURLPath + url.PathEscape(token)
}

var (
	ErrAnnouncementNotFound = errors.New("announcement not found")
	ErrAnnouncementExpired  = errors.New("announcement expired")
)
