package shortlink

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, link *ShortLink) error
	GetByToken(ctx context.Context, token string) (*ShortLink, error)
	GetByTarget(ctx context.Context, targetKind, targetToken string) (*ShortLink, error)
	UpdateExpiresAt(ctx context.Context, id string, expiresAt *time.Time) error
	ListMissingExpiresAt(ctx context.Context, limit int) ([]ShortLink, error)
	LookupKnownTargetExpiresAt(ctx context.Context, targetKind, targetToken string) (*time.Time, bool, error)
	ListExpiredIDs(ctx context.Context, before time.Time, limit int) ([]string, error)
	DeleteByIDs(ctx context.Context, ids []string) (int64, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, link *ShortLink) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Create(link).Error
}

func (r *repository) GetByToken(ctx context.Context, token string) (*ShortLink, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var link ShortLink
	err := r.db.WithContext(ctx).First(&link, "short_token = ?", normalizeToken(token)).Error
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *repository) GetByTarget(ctx context.Context, targetKind, targetToken string) (*ShortLink, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var link ShortLink
	err := r.db.WithContext(ctx).
		Where("target_kind = ? AND target_token = ?", normalizeTargetKind(targetKind), strings.TrimSpace(targetToken)).
		First(&link).Error
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *repository) UpdateExpiresAt(ctx context.Context, id string, expiresAt *time.Time) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).
		Model(&ShortLink{}).
		Where("id = ?", strings.TrimSpace(id)).
		Update("expires_at", expiresAt).Error
}

func (r *repository) ListMissingExpiresAt(ctx context.Context, limit int) ([]ShortLink, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var links []ShortLink
	err := r.db.WithContext(ctx).
		Model(&ShortLink{}).
		Where("expires_at IS NULL AND target_kind IN ?", []string{TargetKindApprovalForm, TargetKindWorkflowAnnouncement}).
		Order("created_at ASC").
		Limit(limit).
		Find(&links).Error
	return links, err
}

func (r *repository) LookupKnownTargetExpiresAt(ctx context.Context, targetKind, targetToken string) (*time.Time, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	targetKind = normalizeTargetKind(targetKind)
	targetToken = strings.TrimSpace(targetToken)
	if targetToken == "" {
		return nil, false, nil
	}
	switch targetKind {
	case TargetKindApprovalForm:
		expiresAt, found, err := r.lookupApprovalFormExpiresAt(ctx, targetToken)
		if err != nil || found {
			return expiresAt, found, err
		}
		return r.lookupLegacyApprovalRecipientFormExpiresAt(ctx, targetToken)
	case TargetKindWorkflowAnnouncement:
		return r.lookupAnnouncementExpiresAt(ctx, targetToken)
	default:
		return nil, false, nil
	}
}

type targetExpirationRow struct {
	ExpirationTime *time.Time `gorm:"column:expiration_time"`
}

func (r *repository) lookupApprovalFormExpiresAt(ctx context.Context, targetToken string) (*time.Time, bool, error) {
	var row targetExpirationRow
	err := r.db.WithContext(ctx).
		Table("workflow_approval_forms").
		Select("expiration_time").
		Where("access_token = ?", targetToken).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return cloneTime(row.ExpirationTime), true, nil
}

func (r *repository) lookupLegacyApprovalRecipientFormExpiresAt(ctx context.Context, targetToken string) (*time.Time, bool, error) {
	var row targetExpirationRow
	err := r.db.WithContext(ctx).
		Table("workflow_approval_recipients AS recipients").
		Select("forms.expiration_time").
		Joins("JOIN workflow_approval_forms AS forms ON forms.id = recipients.form_id").
		Where("recipients.access_token = ?", targetToken).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return cloneTime(row.ExpirationTime), true, nil
}

func (r *repository) lookupAnnouncementExpiresAt(ctx context.Context, targetToken string) (*time.Time, bool, error) {
	var row targetExpirationRow
	err := r.db.WithContext(ctx).
		Table("announcements").
		Select("expiration_time").
		Where("access_token = ?", targetToken).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return cloneTime(row.ExpirationTime), true, nil
}

func (r *repository) ListExpiredIDs(ctx context.Context, before time.Time, limit int) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&ShortLink{}).
		Where("expires_at IS NOT NULL AND expires_at <= ?", before).
		Order("expires_at ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *repository) DeleteByIDs(ctx context.Context, ids []string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&ShortLink{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func isTokenConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "short_token") &&
		(strings.Contains(message, "unique") ||
			strings.Contains(message, "duplicate") ||
			strings.Contains(message, "duplicated"))
}

func isTargetConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "target") &&
		(strings.Contains(message, "unique") ||
			strings.Contains(message, "duplicate") ||
			strings.Contains(message, "duplicated"))
}
