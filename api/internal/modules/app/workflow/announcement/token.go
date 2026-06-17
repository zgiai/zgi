package announcement

import (
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

var newAnnouncementToken = generateToken

func isAnnouncementTokenConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "access_token") {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
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
			return "", fmt.Errorf("generate announcement token: %w", err)
		}
		builder.WriteByte(tokenAlphabet[index.Int64()])
	}
	return builder.String(), nil
}
