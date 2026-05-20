package tool_file

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
)

type ToolFileURLMode string

const (
	ToolFileURLModeSigned    ToolFileURLMode = "signed"
	ToolFileURLModePermanent ToolFileURLMode = "permanent"
)

// FileSignature handles file signature generation and verification
type FileSignature struct {
	secretKey     []byte
	accessTimeout int
	filesURL      string
	internalURL   string
}

// NewFileSignature creates a new file signature handler
func NewFileSignature(cfg *config.Config) *FileSignature {
	secretKey := []byte(cfg.App.SecretKey)

	accessTimeout := cfg.App.FilesAccessTimeout
	if accessTimeout <= 0 {
		accessTimeout = 3600
	}

	return &FileSignature{
		secretKey:     secretKey,
		accessTimeout: accessTimeout,
		filesURL:      cfg.App.FilesURL,
		internalURL:   cfg.App.InternalFilesURL,
	}
}

// SignToolFile generates a signed URL for tool file access
// It signs tool-file URLs with nonce and expiry
func (fs *FileSignature) SignToolFile(toolFileID, extension string) (string, error) {
	return fs.SignToolFileWithMode(toolFileID, extension, ToolFileURLModeSigned)
}

func (fs *FileSignature) SignToolFileWithMode(toolFileID, extension string, mode ToolFileURLMode) (string, error) {
	if len(fs.secretKey) == 0 {
		return "", fmt.Errorf("tool file signing secret is not configured")
	}

	baseURL := fs.filesURL
	if baseURL == "" {
		baseURL = fs.internalURL
	}

	filePreviewURL := fmt.Sprintf("%s/console/api/files/tools/%s%s", strings.TrimRight(baseURL, "/"), toolFileID, extension)

	expiresAt := int64(0)
	if normalizeToolFileURLMode(mode) == ToolFileURLModeSigned {
		expiresAt = time.Now().Unix() + int64(fs.accessTimeout)
	}
	expiresAtStr := strconv.FormatInt(expiresAt, 10)

	// Generate random nonce
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := fmt.Sprintf("%x", nonceBytes)

	// Create signature
	dataToSign := fmt.Sprintf("tool-file|%s|%s|%s", toolFileID, expiresAtStr, nonce)
	signature := hmac.New(sha256.New, fs.secretKey)
	signature.Write([]byte(dataToSign))
	encodedSign := base64.URLEncoding.EncodeToString(signature.Sum(nil))

	return fmt.Sprintf("%s?expires_at=%s&nonce=%s&sign=%s", filePreviewURL, expiresAtStr, nonce, encodedSign), nil
}

// VerifyToolFileSignature verifies the signature of a tool file request
// It rebuilds the signed message and compares the HMAC signature
func (fs *FileSignature) VerifyToolFileSignature(fileID, timestamp, nonce, sign string) bool {
	if len(fs.secretKey) == 0 {
		return false
	}

	// Recreate the signature
	dataToSign := fmt.Sprintf("tool-file|%s|%s|%s", fileID, timestamp, nonce)
	signature := hmac.New(sha256.New, fs.secretKey)
	signature.Write([]byte(dataToSign))
	recalculatedSign := base64.URLEncoding.EncodeToString(signature.Sum(nil))

	// Verify signature
	if sign != recalculatedSign {
		return false
	}

	// Check timestamp
	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	currentTime := time.Now().Unix()
	return currentTime-timestampInt <= int64(fs.accessTimeout)
}

func (fs *FileSignature) VerifyToolFileSignatureWithExpiry(fileID, expiresAt, nonce, sign string) bool {
	if len(fs.secretKey) == 0 {
		return false
	}

	// Recreate the signature
	dataToSign := fmt.Sprintf("tool-file|%s|%s|%s", fileID, expiresAt, nonce)
	signature := hmac.New(sha256.New, fs.secretKey)
	signature.Write([]byte(dataToSign))
	recalculatedSign := base64.URLEncoding.EncodeToString(signature.Sum(nil))

	// Verify signature
	if sign != recalculatedSign {
		return false
	}

	expiresAtInt, err := strconv.ParseInt(expiresAt, 10, 64)
	if err != nil {
		return false
	}

	if expiresAtInt == 0 {
		return true
	}

	currentTime := time.Now().Unix()
	return currentTime <= expiresAtInt
}

// SignPluginFile generates a signed URL for plugin file upload
func (fs *FileSignature) SignPluginFile(filename, mimetype, tenantID, userID string) (string, error) {
	if len(fs.secretKey) == 0 {
		return "", fmt.Errorf("tool file signing secret is not configured")
	}

	// Use internal URL for plugin/tool file access
	baseURL := fs.internalURL
	if baseURL == "" {
		baseURL = fs.filesURL
	}

	url := fmt.Sprintf("%s/files/upload/for-plugin", baseURL)

	if userID == "" {
		userID = "DEFAULT-USER"
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Generate random nonce
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := fmt.Sprintf("%x", nonceBytes)

	// Create signature
	dataToSign := fmt.Sprintf("upload|%s|%s|%s|%s|%s|%s", filename, mimetype, tenantID, userID, timestamp, nonce)
	signature := hmac.New(sha256.New, fs.secretKey)
	signature.Write([]byte(dataToSign))
	encodedSign := base64.URLEncoding.EncodeToString(signature.Sum(nil))

	return fmt.Sprintf("%s?timestamp=%s&nonce=%s&sign=%s&user_id=%s&tenant_id=%s",
		url, timestamp, nonce, encodedSign, userID, tenantID), nil
}

// VerifyPluginFileSignature verifies the signature of a plugin file upload request
func (fs *FileSignature) VerifyPluginFileSignature(filename, mimetype, tenantID, userID, timestamp, nonce, sign string) bool {
	if len(fs.secretKey) == 0 {
		return false
	}

	if userID == "" {
		userID = "DEFAULT-USER"
	}

	// Recreate the signature
	dataToSign := fmt.Sprintf("upload|%s|%s|%s|%s|%s|%s", filename, mimetype, tenantID, userID, timestamp, nonce)
	signature := hmac.New(sha256.New, fs.secretKey)
	signature.Write([]byte(dataToSign))
	recalculatedSign := base64.URLEncoding.EncodeToString(signature.Sum(nil))

	// Verify signature
	if sign != recalculatedSign {
		return false
	}

	// Check timestamp
	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	currentTime := time.Now().Unix()
	return currentTime-timestampInt <= int64(fs.accessTimeout)
}

// Global file signature instance
var GlobalFileSignature *FileSignature

// InitFileSignature initializes the global file signature instance
func InitFileSignature(cfg *config.Config) {
	GlobalFileSignature = NewFileSignature(cfg)
}

// SignToolFileGlobal is a global function for signing tool files
func SignToolFileGlobal(toolFileID, extension string) (string, error) {
	if GlobalFileSignature == nil {
		return "", fmt.Errorf("file signature not initialized")
	}
	return GlobalFileSignature.SignToolFileWithMode(toolFileID, extension, ToolFileURLModeSigned)
}

func SignToolFileGlobalWithMode(toolFileID, extension string, mode ToolFileURLMode) (string, error) {
	if GlobalFileSignature == nil {
		return "", fmt.Errorf("file signature not initialized")
	}
	return GlobalFileSignature.SignToolFileWithMode(toolFileID, extension, mode)
}

// VerifyToolFileSignatureGlobal is a global function for verifying tool file signatures
func VerifyToolFileSignatureGlobal(fileID, timestamp, nonce, sign string) bool {
	if GlobalFileSignature == nil {
		return false
	}
	return GlobalFileSignature.VerifyToolFileSignature(fileID, timestamp, nonce, sign)
}

func VerifyToolFileSignatureWithExpiryGlobal(fileID, expiresAt, nonce, sign string) bool {
	if GlobalFileSignature == nil {
		return false
	}
	return GlobalFileSignature.VerifyToolFileSignatureWithExpiry(fileID, expiresAt, nonce, sign)
}

func normalizeToolFileURLMode(mode ToolFileURLMode) ToolFileURLMode {
	if mode == ToolFileURLModePermanent {
		return ToolFileURLModePermanent
	}
	return ToolFileURLModeSigned
}
