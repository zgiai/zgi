package announcement

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

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateOrGetRuntimeAnnouncement(ctx context.Context, params CreateRuntimeAnnouncementParams) (*RuntimeAnnouncement, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("announcement service is not initialized")
	}
	if err := validateRuntimeParams(params); err != nil {
		return nil, err
	}

	var existing Announcement
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND workflow_run_id = ? AND node_id = ?", params.TenantID, params.WorkflowRunID, params.NodeID).
		First(&existing).Error
	if err == nil {
		return s.runtimeAnnouncementPayload(&existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load announcement: %w", err)
	}

	announcement, err := buildRuntimeAnnouncement(params)
	if err != nil {
		return nil, err
	}
	if err := s.createRuntimeAnnouncementWithTokenRetry(ctx, announcement); err != nil {
		return nil, err
	}
	return s.runtimeAnnouncementPayload(announcement), nil
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

func (s *Service) createRuntimeAnnouncementWithTokenRetry(ctx context.Context, announcement *Announcement) error {
	var createErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		if attempt > 0 {
			token, err := newAnnouncementToken()
			if err != nil {
				return err
			}
			announcement.AccessToken = token
		}
		createErr = s.db.WithContext(ctx).Create(announcement).Error
		if createErr == nil {
			return nil
		}
		if !isAnnouncementTokenConflict(createErr) {
			return fmt.Errorf("create announcement: %w", createErr)
		}
	}
	return fmt.Errorf("create announcement after token retries: %w", createErr)
}

func (s *Service) runtimeAnnouncementPayload(announcement *Announcement) *RuntimeAnnouncement {
	payload := announcementPayload(announcement)
	return &RuntimeAnnouncement{
		Announcement: announcement,
		Payload:      payload,
	}
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
		NodeID:       announcement.NodeID,
		NodeTitle:    announcement.NodeTitle,
		Content:      announcement.RenderedContent,
		ExpirationAt: announcement.ExpirationTime.Unix(),
		Expired:      time.Now().After(announcement.ExpirationTime),
		URL:          announcementURL(announcement.AccessToken),
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
	return ValidateConfig(params.Config)
}

func ValidateConfig(config NodeConfig) error {
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

func announcementURL(token string) string {
	base := strings.TrimRight(appconfig.Current().Console.WebURL, "/")
	if base == "" {
		base = strings.TrimRight(appconfig.Current().Email.ConsoleWebURL, "/")
	}
	return base + announcementURLPath + url.PathEscape(token)
}

var (
	ErrAnnouncementNotFound = errors.New("announcement not found")
	ErrAnnouncementExpired  = errors.New("announcement expired")
)
