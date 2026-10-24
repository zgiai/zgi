package tool_file

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/pkg/response"
)

const channelDeliveryQueryKey = "channel_delivery"
const maxChannelDeliveryURLTTL = time.Duration(config.MaxChannelDeliveryFileTTLSeconds) * time.Second

// SignChannelDeliveryFileGlobal explicitly shares one verified channel-delivery
// artifact through the existing file download endpoint. The caller must first
// check the live delivery route and file ownership; protected artifacts also
// require frozen run ownership. This finite bearer grant does not change file
// ownership or authorize ordinary signed URLs.
// ChannelDeliveryWithdrawn, when set, reports whether a channel-delivered
// file was withdrawn since it was sent, such as by archiving the artifact
// it became; its download link then stops working.
var ChannelDeliveryWithdrawn func(ctx context.Context, fileID string) bool

func SignChannelDeliveryFileGlobal(file *ToolFile, deliveryID, extension string) (string, error) {
	if GlobalFileSignature == nil {
		return "", fmt.Errorf("file signature not initialized")
	}
	return GlobalFileSignature.signChannelDeliveryFile(file, deliveryID, extension)
}

func channelDeliveryFile(file *ToolFile) bool {
	if file == nil || file.DeletedAt != nil || file.IsExpired(time.Now()) || !validFileUUID(file.ID) {
		return false
	}
	// Protected execution artifacts require their frozen execution ownership.
	// Historical unprotected channel artifacts are already scope-checked by the
	// linker before this purpose-bound grant is issued.
	i := file.ExecutionIdentity
	if i == nil && file.ExecutionMessageID == nil {
		return true
	}
	if i == nil || file.ExecutionMessageID == nil || file.ConversationID == nil {
		return false
	}
	if i.Version != execution.BackgroundIdentityVersion || i.ApplicationType != "digital_human" || (i.Entry != "channel" && i.Entry != "task") {
		return false
	}
	access := ExecutionFileAccess{OrganizationID: file.TenantID, SubjectID: file.UserID, WorkspaceID: i.WorkspaceID,
		ApplicationType: i.ApplicationType, ApplicationID: i.ApplicationID, Entry: i.Entry,
		SubjectKind: i.SubjectKind, ConversationID: *file.ConversationID}
	return access.owns(file)
}

func (fs *FileSignature) signChannelDeliveryFile(file *ToolFile, deliveryID, extension string) (string, error) {
	if len(fs.secretKey) == 0 || !validFileUUID(deliveryID) || !channelDeliveryFile(file) {
		return "", ErrExecutionFileAccess
	}
	baseURL := fs.filesURL
	if baseURL == "" {
		baseURL = fs.internalURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || strings.ContainsAny(extension, "/\\?&#\r\n") {
		return "", fmt.Errorf("channel file download origin or extension is invalid")
	}
	ttl := min(fs.channelDeliveryTimeout, maxChannelDeliveryURLTTL)
	if ttl <= 0 {
		return "", ErrExecutionFileAccess
	}
	expires := time.Now().Add(ttl)
	if file.ExpiresAt != nil && file.ExpiresAt.Before(expires) {
		expires = *file.ExpiresAt
	}
	expiry := strconv.FormatInt(expires.Unix(), 10)
	if expires.Unix() <= time.Now().Unix() {
		return "", ErrExecutionFileAccess
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("generate channel download nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)
	query := url.Values{channelDeliveryQueryKey: {deliveryID}, "expires_at": {expiry}, "nonce": {nonce},
		"sign": {base64.URLEncoding.EncodeToString(fs.channelDeliveryMAC(file.ID, deliveryID, expiry, nonce))}, "download": {"1"}}
	return strings.TrimRight(baseURL, "/") + "/console/api/files/tools/" + file.ID + extension + "?" + query.Encode(), nil
}

func (fs *FileSignature) channelDeliveryMAC(fileID, deliveryID, expiry, nonce string) []byte {
	mac := hmac.New(sha256.New, fs.secretKey)
	fmt.Fprintf(mac, "channel-delivery-file|%s|%s|%s|%s", fileID, deliveryID, expiry, nonce)
	return mac.Sum(nil)
}

func (h *HTTPHandler) serveChannelDeliveryFile(c *gin.Context, fileID string) {
	deny := func() { response.Fail(c, response.ErrFileNotFound) }
	fs := h.signature
	if fs == nil && config.GlobalConfig != nil {
		fs = NewFileSignature(config.GlobalConfig)
	}
	deliveryID, expiry, nonce := c.Query(channelDeliveryQueryKey), c.Query("expires_at"), c.Query("nonce")
	expires, err := strconv.ParseInt(expiry, 10, 64)
	now := time.Now().Unix()
	signature, decodeErr := base64.URLEncoding.DecodeString(c.Query("sign"))
	if fs == nil || len(fs.secretKey) == 0 || !validFileUUID(fileID) || !validFileUUID(deliveryID) ||
		err != nil || decodeErr != nil || expires <= now || expires > now+int64(maxChannelDeliveryURLTTL/time.Second) ||
		len(nonce) != 32 || !shouldDownloadToolFile(c.Query("download")) ||
		!hmac.Equal(signature, fs.channelDeliveryMAC(fileID, deliveryID, expiry, nonce)) {
		deny()
		return
	}
	file, err := h.manager.GetToolFileByID(c.Request.Context(), fileID)
	if err != nil || !channelDeliveryFile(file) {
		deny()
		return
	}
	if withdrawn := ChannelDeliveryWithdrawn; withdrawn != nil && withdrawn(c.Request.Context(), fileID) {
		deny()
		return
	}
	// Only this purpose-bound grant may read a channel-delivery file here. Keep
	// GetFileBinary and ordinary signed redirects deny-by-default for protected
	// execution artifacts and unrelated callers.
	data, err := h.manager.storage.Load(file.FileKey)
	if err != nil {
		deny()
		return
	}
	WriteExecutionFile(c, file, data)
}
