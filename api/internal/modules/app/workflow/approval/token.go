package approval

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"gorm.io/gorm"
)

const (
	tokenLength            = 8
	tokenAlphabet          = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	tokenCreateMaxAttempts = 5
)

var newApprovalToken = generateToken

func (s *Service) createRuntimeFormWithTokenRetry(ctx context.Context, form *Form, deliveries []Delivery, recipients []Recipient) error {
	var createErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		if attempt > 0 {
			if err := regenerateRecipientTokens(recipients); err != nil {
				return err
			}
		}
		createErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(form).Error; err != nil {
				return fmt.Errorf("create approval form: %w", err)
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
	return fmt.Errorf("create approval recipients after token retries: %w", createErr)
}

func regenerateRecipientTokens(recipients []Recipient) error {
	for i := range recipients {
		token, err := newApprovalToken()
		if err != nil {
			return err
		}
		recipients[i].AccessToken = token
	}
	return nil
}

func isApprovalTokenConflict(err error) bool {
	if err == nil {
		return false
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
