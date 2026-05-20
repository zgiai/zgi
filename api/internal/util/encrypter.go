package util

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"

	// "github.com/zgiai/ginext/internal/libs"
	// "github.com/zgiai/ginext/internal/model"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/security"
)

// obfuscatedToken obfuscates a token for display purposes
func ObfuscatedToken(token string) string {
	if token == "" {
		return token
	}
	if len(token) <= 8 {
		return "********************"
	}
	return token[:6] + "************" + token[len(token)-2:]
}

// encryptToken encrypts a token using tenant's public key
func EncryptToken(tenantID string, token string) (string, error) {
	var tenant model.Workspace
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return "", fmt.Errorf("tenant with id %s not found", tenantID)
	}

	if tenant.EncryptPublicKey == nil {
		return "", fmt.Errorf("tenant %s has no encryption public key", tenantID)
	}

	// TODO: RSA
	// encryptedToken, err := rsa.Encrypt(token, *tenant.EncryptPublicKey)
	// if err != nil {
	// 	return "", err
	// }

	return base64.StdEncoding.EncodeToString([]byte(token)), nil
}

// decryptToken decrypts a token using tenant-specific encryption
func DecryptToken(tenantID string, token string) (string, error) {
	var tenant model.Workspace
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return "", fmt.Errorf("tenant with id %s not found", tenantID)
	}

	if tenant.EncryptPublicKey == nil {
		return "", fmt.Errorf("tenant %s has no encryption public key", tenantID)
	}

	// decodedToken, err := base64.StdEncoding.DecodeString(token)
	// if err != nil {
	// 	return "", err
	// }

	// TODO: RSA
	// return rsa.Decrypt(decodedToken, tenantID), nil

	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", err
	}
	return string(decodedToken), nil
}

// batchDecryptToken decrypts multiple tokens at once
func BatchDecryptToken(tenantID string, tokens []string) ([]string, error) {
	var tenant model.Workspace
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, fmt.Errorf("tenant with id %s not found", tenantID)
	}

	if tenant.EncryptPublicKey == nil {
		return nil, fmt.Errorf("tenant %s has no encryption public key", tenantID)
	}

	// rsaKey, cipherRSA := getDecryptDecoding(tenantID)

	// var result []string
	// for _, token := range tokens {
	// 	decodedToken, err := base64.StdEncoding.DecodeString(token)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	decrypted := decryptTokenWithDecoding(decodedToken, rsaKey, cipherRSA)
	// 	result = append(result, decrypted)
	// }

	var result []string
	for _, token := range tokens {
		decodedToken, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return nil, err
		}
		result = append(result, string(decodedToken))
	}
	return result, nil
}

// getDecryptDecoding gets the decrypt decoding information
func GetDecryptDecoding(tenantID string) (*rsa.PrivateKey, error) {
	var tenant model.Workspace
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, fmt.Errorf("tenant with id %s not found", tenantID)
	}

	if tenant.EncryptPublicKey == nil {
		return nil, fmt.Errorf("tenant %s has no encryption public key", tenantID)
	}

	return security.GetDecryptDecoding(tenantID)
}

// decryptTokenWithDecoding decrypts a token with provided decoding information
func DecryptTokenWithDecoding(token string, privateKey *rsa.PrivateKey) (string, error) {
	decodedToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", err
	}

	return security.DecryptTokenWithDecoding(decodedToken, privateKey)
}
