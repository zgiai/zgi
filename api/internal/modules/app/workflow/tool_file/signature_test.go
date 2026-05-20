package tool_file

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/config"
)

func TestFileSignature_SignToolFile_DefaultSignedModeUsesExpiresAt(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey:        "test-secret-key",
			FilesURL:         "http://example.com",
			InternalFilesURL: "http://internal.example.com",
		},
	}

	fs := NewFileSignature(cfg)

	toolFileID := "test-file-id"
	extension := ".txt"

	signedURL, err := fs.SignToolFile(toolFileID, extension)
	require.NoError(t, err)

	assert.Contains(t, signedURL, "http://example.com/console/api/files/tools/")
	assert.Contains(t, signedURL, toolFileID)
	assert.Contains(t, signedURL, extension)
	assert.Contains(t, signedURL, "expires_at=")
	assert.Contains(t, signedURL, "nonce=")
	assert.Contains(t, signedURL, "sign=")
	assert.NotContains(t, signedURL, "timestamp=")

	parsed, err := url.Parse(signedURL)
	require.NoError(t, err)
	expiresAt, err := strconv.ParseInt(parsed.Query().Get("expires_at"), 10, 64)
	require.NoError(t, err)
	assert.Greater(t, expiresAt, time.Now().Unix())
}

func TestFileSignature_SignToolFileWithMode_PermanentUsesZeroExpiry(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey:        "test-secret-key",
			FilesURL:         "http://example.com",
			InternalFilesURL: "http://internal.example.com",
		},
	}

	fs := NewFileSignature(cfg)

	signedURL, err := fs.SignToolFileWithMode("test-file-id", ".txt", ToolFileURLModePermanent)
	require.NoError(t, err)

	parsed, err := url.Parse(signedURL)
	require.NoError(t, err)
	assert.Equal(t, "0", parsed.Query().Get("expires_at"))
}

func TestFileSignature_RejectsMissingSecret(t *testing.T) {
	fs := NewFileSignature(&config.Config{})

	signedURL, err := fs.SignToolFile("test-file-id", ".txt")
	require.Error(t, err)
	assert.Empty(t, signedURL)

	pluginURL, err := fs.SignPluginFile("test.txt", "text/plain", "tenant123", "user456")
	require.Error(t, err)
	assert.Empty(t, pluginURL)

	assert.False(t, fs.VerifyToolFileSignature("test-file-id", "0", "nonce", "sign"))
	assert.False(t, fs.VerifyToolFileSignatureWithExpiry("test-file-id", "0", "nonce", "sign"))
	assert.False(t, fs.VerifyPluginFileSignature("test.txt", "text/plain", "tenant123", "user456", "0", "nonce", "sign"))
}

func TestFileSignature_VerifyToolFileSignature(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey:        "test-secret-key",
			FilesURL:         "http://example.com",
			InternalFilesURL: "http://internal.example.com",
		},
	}

	fs := NewFileSignature(cfg)

	toolFileID := "test-file-id"
	extension := ".txt"

	// Generate a signed URL
	signedURL, err := fs.SignToolFile(toolFileID, extension)
	require.NoError(t, err)
	_ = signedURL // Use the variable to avoid unused variable error

	// Parse the URL to extract signature components
	// For testing, we'll manually create valid signature components
	timestamp := fmt.Sprintf("%d", time.Now().Unix()) // Use current timestamp
	nonce := "testnonce12345678"

	// Create expected signature
	dataToSign := fmt.Sprintf("tool-file|%s|%s|%s", toolFileID, timestamp, nonce)
	expectedSign := generateTestSignature(dataToSign, cfg.App.SecretKey)

	// Test valid signature
	isValid := fs.VerifyToolFileSignature(toolFileID, timestamp, nonce, expectedSign)
	assert.True(t, isValid)

	// Test invalid signature
	invalidSign := "invalid-signature"
	isValid = fs.VerifyToolFileSignature(toolFileID, timestamp, nonce, invalidSign)
	assert.False(t, isValid)
}

func TestFileSignature_VerifyToolFileSignatureWithExpiry_RejectsTamperedExpiry(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey:        "test-secret-key",
			FilesURL:         "http://example.com",
			InternalFilesURL: "http://internal.example.com",
		},
	}

	fs := NewFileSignature(cfg)

	signedURL, err := fs.SignToolFileWithMode("test-file-id", ".txt", ToolFileURLModeSigned)
	require.NoError(t, err)

	parsed, err := url.Parse(signedURL)
	require.NoError(t, err)
	query := parsed.Query()

	assert.True(t, fs.VerifyToolFileSignatureWithExpiry("test-file-id", query.Get("expires_at"), query.Get("nonce"), query.Get("sign")))
	assert.False(t, fs.VerifyToolFileSignatureWithExpiry("test-file-id", "0", query.Get("nonce"), query.Get("sign")))
}

func TestFileSignature_VerifyToolFileSignatureWithExpiry_Expired(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey: "test-secret-key",
		},
	}

	fs := NewFileSignature(cfg)

	expiresAt := time.Now().Add(-2 * time.Hour).Unix()
	expiresAtStr := fmt.Sprintf("%d", expiresAt)
	nonce := "testnonce12345678"
	dataToSign := fmt.Sprintf("tool-file|%s|%s|%s", "test-file-id", expiresAtStr, nonce)
	validSign := generateTestSignature(dataToSign, cfg.App.SecretKey)

	assert.False(t, fs.VerifyToolFileSignatureWithExpiry("test-file-id", expiresAtStr, nonce, validSign))
}

func TestFileSignature_VerifyToolFileSignature_ExpiredTimestamp(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey: "test-secret-key",
		},
	}

	fs := NewFileSignature(cfg)

	toolFileID := "test-file-id"
	nonce := "testnonce12345678"

	// Use timestamp from 2 hours ago (expired)
	expiredTimestamp := time.Now().Add(-2 * time.Hour).Unix()
	timestampStr := fmt.Sprintf("%d", expiredTimestamp)

	// Create valid signature for expired timestamp
	dataToSign := fmt.Sprintf("tool-file|%s|%s|%s", toolFileID, timestampStr, nonce)
	validSign := generateTestSignature(dataToSign, cfg.App.SecretKey)

	// Should be invalid due to expired timestamp
	isValid := fs.VerifyToolFileSignature(toolFileID, timestampStr, nonce, validSign)
	assert.False(t, isValid)
}

func TestFileSignature_SignPluginFile(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey:        "test-secret-key",
			FilesURL:         "http://example.com",
			InternalFilesURL: "http://internal.example.com",
		},
	}

	fs := NewFileSignature(cfg)

	filename := "test.txt"
	mimetype := "text/plain"
	tenantID := "tenant123"
	userID := "user456"

	signedURL, err := fs.SignPluginFile(filename, mimetype, tenantID, userID)
	require.NoError(t, err)

	assert.Contains(t, signedURL, "http://internal.example.com/files/upload/for-plugin")
	assert.Contains(t, signedURL, "timestamp=")
	assert.Contains(t, signedURL, "nonce=")
	assert.Contains(t, signedURL, "sign=")
	assert.Contains(t, signedURL, "user_id="+userID)
	assert.Contains(t, signedURL, "tenant_id="+tenantID)
}

func TestFileSignature_VerifyPluginFileSignature(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey: "test-secret-key",
		},
	}

	fs := NewFileSignature(cfg)

	filename := "test.txt"
	mimetype := "text/plain"
	tenantID := "tenant123"
	userID := "user456"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := "testnonce12345678"

	// Create expected signature
	dataToSign := fmt.Sprintf("upload|%s|%s|%s|%s|%s|%s", filename, mimetype, tenantID, userID, timestamp, nonce)
	expectedSign := generateTestSignature(dataToSign, cfg.App.SecretKey)

	// Test valid signature
	isValid := fs.VerifyPluginFileSignature(filename, mimetype, tenantID, userID, timestamp, nonce, expectedSign)
	assert.True(t, isValid)

	// Test invalid signature
	invalidSign := "invalid-signature"
	isValid = fs.VerifyPluginFileSignature(filename, mimetype, tenantID, userID, timestamp, nonce, invalidSign)
	assert.False(t, isValid)
}

func TestFileSignature_EmptyUserID(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey: "test-secret-key",
		},
	}

	fs := NewFileSignature(cfg)

	filename := "test.txt"
	mimetype := "text/plain"
	tenantID := "tenant123"
	userID := "" // Empty user ID should become "DEFAULT-USER"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := "testnonce12345678"

	// Create expected signature with DEFAULT-USER
	dataToSign := fmt.Sprintf("upload|%s|%s|%s|DEFAULT-USER|%s|%s", filename, mimetype, tenantID, timestamp, nonce)
	expectedSign := generateTestSignature(dataToSign, cfg.App.SecretKey)

	// Test verification with empty userID
	isValid := fs.VerifyPluginFileSignature(filename, mimetype, tenantID, userID, timestamp, nonce, expectedSign)
	assert.True(t, isValid)
}

func TestGlobalFileSignatureFunctions(t *testing.T) {
	// Test global functions without initialization
	signedURL, err := SignToolFileGlobal("test-file", ".txt")
	assert.Error(t, err)
	assert.Empty(t, signedURL)

	isValid := VerifyToolFileSignatureGlobal("test-file", "123", "nonce", "sign")
	assert.False(t, isValid)

	// Initialize global signature
	cfg := &config.Config{
		App: config.AppConfig{
			SecretKey: "test-secret-key",
		},
	}
	InitFileSignature(cfg)

	// Test global functions with initialization
	signedURL, err = SignToolFileGlobal("test-file", ".txt")
	assert.NoError(t, err)
	assert.NotEmpty(t, signedURL)

	permanentURL, err := SignToolFileGlobalWithMode("test-file", ".txt", ToolFileURLModePermanent)
	assert.NoError(t, err)
	parsed, parseErr := url.Parse(permanentURL)
	assert.NoError(t, parseErr)
	assert.Equal(t, "0", parsed.Query().Get("expires_at"))
}

// Helper function to generate test signatures
func generateTestSignature(dataToSign, secretKey string) string {
	signature := hmac.New(sha256.New, []byte(secretKey))
	signature.Write([]byte(dataToSign))
	return base64.URLEncoding.EncodeToString(signature.Sum(nil))
}
