package approval

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
	"gorm.io/gorm"
)

const (
	tokenLength            = 8
	tokenAlphabet          = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	tokenCreateMaxAttempts = 5
)

var newApprovalToken = generateToken

var errApprovalTokenConflict = errors.New("approval access_token conflict")

func (s *Service) createRuntimeFormWithTokenRetry(ctx context.Context, form *Form, deliveries []Delivery, recipients []Recipient) error {
	var createErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		if attempt > 0 {
			token, err := newApprovalToken()
			if err != nil {
				return err
			}
			form.AccessToken = token
		}
		createErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ensureApprovalFormTokenAvailable(ctx, tx, form.AccessToken); err != nil {
				return err
			}
			if err := tx.Create(form).Error; err != nil {
				return fmt.Errorf("create approval form: %w", err)
			}
			if err := createApprovalFormShortLink(ctx, tx, form.AccessToken, form.ExpirationTime); err != nil {
				return err
			}
			if len(deliveries) > 0 {
				if err := tx.Create(&deliveries).Error; err != nil {
					return fmt.Errorf("create approval deliveries: %w", err)
				}
			}
			if len(recipients) > 0 {
				if err := tx.Create(&recipients).Error; err != nil {
					return fmt.Errorf("create approval recipients: %w", err)
				}
			}
			return nil
		})
		if createErr == nil {
			return nil
		}
		if !isApprovalTokenConflict(createErr) {
			return createErr
		}
	}
	return fmt.Errorf("create approval form after token retries: %w", createErr)
}

func createApprovalFormShortLink(ctx context.Context, db *gorm.DB, accessToken string, expiresAt time.Time) error {
	shortLinkService := shortlinkcap.NewServiceWithDB(db)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return fmt.Errorf("approval form access token is required")
	}
	if _, err := shortLinkService.CreateOrGet(ctx, shortlinkcap.CreateOrGetRequest{
		TargetKind:  shortlinkcap.TargetKindApprovalForm,
		TargetToken: accessToken,
		TargetPath:  approvalTargetPath(accessToken),
		ExpiresAt:   &expiresAt,
	}); err != nil {
		return fmt.Errorf("create approval short link: %w", err)
	}
	return nil
}

func isApprovalTokenConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errApprovalTokenConflict) {
		return true
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "access_token") {
		return false
	}
	return strings.Contains(message, "unique") ||
		strings.Contains(message, "duplicate") ||
		strings.Contains(message, "duplicated")
}

func ensureApprovalFormTokenAvailable(ctx context.Context, db *gorm.DB, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("approval form access token is required")
	}
	var count int64
	if err := db.WithContext(ctx).Model(&Recipient{}).Where("access_token = ?", token).Count(&count).Error; err != nil {
		return fmt.Errorf("check legacy approval recipient token: %w", err)
	}
	if count > 0 {
		return errApprovalTokenConflict
	}
	return nil
}

func newApprovalFormToken(ctx context.Context, db *gorm.DB) (string, error) {
	var conflictErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		token, err := newApprovalToken()
		if err != nil {
			return "", err
		}
		if err := ensureApprovalFormTokenAvailable(ctx, db, token); err != nil {
			if isApprovalTokenConflict(err) {
				conflictErr = err
				continue
			}
			return "", err
		}
		return token, nil
	}
	return "", fmt.Errorf("generate approval form access token after token retries: %w", conflictErr)
}

func generateToken() (string, error) {
	var builder strings.Builder
	builder.Grow(tokenLength)
	max := big.NewInt(int64(len(tokenAlphabet)))
	for builder.Len() < tokenLength {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate approval token: %w", err)
		}
		builder.WriteByte(tokenAlphabet[index.Int64()])
	}
	return builder.String(), nil
}
