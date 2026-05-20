package jwt

import (
	"fmt"
	"time"

	"github.com/zgiai/ginext/config"

	"github.com/golang-jwt/jwt/v5"
)

var (
	cfg *config.Config
)

func Init(c *config.Config) {
	cfg = c
}

// GenerateTokenFixed JWT token
// use MapClaims  not RegisteredClaims，make sure payload totally the same with the old online system
func GenerateTokenFixed(userID string, username string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("jwt service not initialized")
	}

	now := time.Now().UTC()
	expTime := now.Add(cfg.JWT.JWTExpire)

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     expTime.Unix(),
		"iss":     cfg.JWT.Issuer,
		"sub":     "Console API Passport",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(cfg.JWT.Secret))
}

func ParseTokenFixed(tokenString string) (map[string]interface{}, error) {
	if cfg == nil {
		return nil, fmt.Errorf("jwt service not initialized")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(cfg.JWT.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return map[string]interface{}(claims), nil
	}

	return nil, fmt.Errorf("invalid token claims")
}

func GetUserIDFromToken(tokenString string) (string, error) {
	claims, err := ParseTokenFixed(tokenString)
	if err != nil {
		return "", err
	}

	if userID, ok := claims["user_id"].(string); ok {
		return userID, nil
	}

	return "", fmt.Errorf("user_id not found in token")
}
