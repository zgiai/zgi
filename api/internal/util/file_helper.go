package util

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
)

// GetSignedFileURL generates a signed URL for file preview
// It signs file-preview URLs with timestamp, nonce, and HMAC-SHA256
// Parameters:
//   - uploadFileID: the file ID to generate URL for
//
// Returns:
//   - signed URL string with timestamp, nonce and signature
//   - error if any
func GetSignedFileURL(uploadFileID string) (string, error) {
	cfg := appconfig.GlobalConfig
	if cfg == nil {
		return "", fmt.Errorf("config not loaded")
	}

	// Generate timestamp (Unix timestamp in seconds)
	timestamp := time.Now().Unix()
	timestampStr := strconv.FormatInt(timestamp, 10)

	// Generate nonce (16 bytes random hex string)
	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Create message to sign: "file-preview|{file_id}|{timestamp}|{nonce}"
	msg := fmt.Sprintf("file-preview|%s|%s|%s", uploadFileID, timestampStr, nonce)

	// Generate HMAC-SHA256 signature
	signature, err := generateFileSignature(msg, cfg.App.SecretKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate signature: %w", err)
	}

	// Build URL with query parameters
	//baseURL := fmt.Sprintf("%s/console/api/files/%s/file-preview", cfg.Console.APIURL, uploadFileID)
	baseURL := fmt.Sprintf("%s/console/api/files/%s/file-preview", cfg.App.FilesURL, uploadFileID)
	params := url.Values{}
	params.Add("timestamp", timestampStr)
	params.Add("nonce", nonce)
	params.Add("sign", signature)

	return fmt.Sprintf("%s?%s", baseURL, params.Encode()), nil
}

// GetSignedFileURLWithConfig generates a signed URL with custom config
// Useful when you want to use a different base URL or secret key
func GetSignedFileURLWithConfig(uploadFileID, filesURL, secretKey string) (string, error) {
	if filesURL == "" || secretKey == "" {
		return "", fmt.Errorf("filesURL and secretKey are required")
	}

	// Generate timestamp (Unix timestamp in seconds)
	timestamp := time.Now().Unix()
	timestampStr := strconv.FormatInt(timestamp, 10)

	// Generate nonce (16 bytes random hex string)
	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Create message to sign: "file-preview|{file_id}|{timestamp}|{nonce}"
	msg := fmt.Sprintf("file-preview|%s|%s|%s", uploadFileID, timestampStr, nonce)

	// Generate HMAC-SHA256 signature
	signature, err := generateFileSignature(msg, secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate signature: %w", err)
	}

	// Build URL with query parameters
	baseURL := fmt.Sprintf("%s/files/%s/file-preview", filesURL, uploadFileID)
	params := url.Values{}
	params.Add("timestamp", timestampStr)
	params.Add("nonce", nonce)
	params.Add("sign", signature)

	return fmt.Sprintf("%s?%s", baseURL, params.Encode()), nil
}

// generateNonce generates a random 16-byte hex string
func generateNonce() (string, error) {
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateFileSignature generates an HMAC-SHA256 signature and encodes it in URL-safe base64
func generateFileSignature(data, secretKey string) (string, error) {
	h := hmac.New(sha256.New, []byte(secretKey))
	_, err := h.Write([]byte(data))
	if err != nil {
		return "", err
	}
	// Use URL-safe base64 so the signature can be passed as a query parameter.
	return base64.URLEncoding.EncodeToString(h.Sum(nil)), nil
}

// VerifyFileSignature verifies the signature for file preview requests
// It rebuilds the signed message and compares the HMAC signature
// Parameters:
//   - uploadFileID: the file ID to verify
//   - timestamp: Unix timestamp string
//   - nonce: random nonce string
//   - sign: signature to verify
//
// Returns:
//   - true if signature is valid and not expired, false otherwise
func VerifyFileSignature(uploadFileID, timestamp, nonce, sign string) bool {
	cfg := appconfig.GlobalConfig
	if cfg == nil {
		return false
	}

	// Recreate the message that was signed: "file-preview|{file_id}|{timestamp}|{nonce}"
	dataToSign := fmt.Sprintf("file-preview|%s|%s|%s", uploadFileID, timestamp, nonce)

	// Generate HMAC-SHA256 signature with secret key
	recalculatedSign, err := generateFileSignature(dataToSign, cfg.App.SecretKey)
	if err != nil {
		return false
	}

	// Verify signature matches
	if sign != recalculatedSign {
		return false
	}

	// Parse timestamp and check if it's within the allowed timeout
	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	currentTime := time.Now().Unix()
	// Check if the request is within the allowed timeout period
	return currentTime-timestampInt <= int64(cfg.App.FilesAccessTimeout)
}
