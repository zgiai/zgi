package handler

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/pkg/response"
)

func (h *PlaygroundHandler) executionFromParseSession(c *gin.Context, sessionID string) (*playgroundExecution, error) {
	if h == nil || h.sessions == nil {
		return nil, newPlaygroundRequestError(response.ErrSystemError, "content parse playground session cache is not initialized")
	}
	exec, ok := h.sessions.Get(sessionID, playgroundRunScope(c))
	if !ok || exec == nil {
		return nil, &playgroundRequestError{code: response.ErrInvalidParam, err: errPlaygroundParseSessionUnavailable}
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return nil, newPlaygroundRequestError(response.ErrNoFileUploaded, "please upload the original file to save this parsed result")
	}
	data, err := readMultipartFile(fileHeader)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrInvalidParam, err: err}
	}
	if got := fileSHA256(data); got != exec.Response.File.SHA256 {
		return nil, newPlaygroundRequestError(response.ErrInvalidParam, "uploaded source file does not match parsed result hash")
	}
	exec.SourceData = data
	exec.SourceMimeType = detectPlaygroundSourceMimeType(fileHeader, data)
	exec.SourceFileExt = normalizePlaygroundFileExt(fileHeader.Filename, exec.SourceMimeType)
	return exec, nil
}

func (h *PlaygroundHandler) storePlaygroundParseSession(c *gin.Context, exec *playgroundExecution) {
	if h == nil || h.sessions == nil || exec == nil {
		return
	}
	exec.Response.ParseSessionID = h.sessions.Store(playgroundRunScope(c), exec)
}

func newPlaygroundParseSessionCache(ttl time.Duration) *playgroundParseSessionCache {
	if ttl <= 0 {
		ttl = playgroundParseSessionTTL
	}
	return &playgroundParseSessionCache{
		ttl:          ttl,
		maxEntries:   playgroundParseSessionMaxEntries,
		maxBytes:     playgroundParseSessionMaxBytes,
		maxItemBytes: playgroundParseSessionMaxItemBytes,
		items:        map[string]playgroundCachedExecution{},
	}
}

func (c *playgroundParseSessionCache) Store(scope service.PlaygroundRunListFilter, exec *playgroundExecution) string {
	if c == nil || exec == nil {
		return ""
	}
	sessionID := newPlaygroundParseSessionID()
	now := time.Now()
	cachedReq := exec.EffectiveRequest
	cachedReq.Data = nil
	cached := playgroundCachedExecution{
		Response:             exec.Response,
		RequestedProviderKey: exec.RequestedProviderKey,
		AdapterName:          exec.AdapterName,
		EffectiveRequest:     cachedReq,
		SourceMimeType:       exec.SourceMimeType,
		SourceFileExt:        exec.SourceFileExt,
		WorkspaceID:          cloneUUIDPointer(scope.WorkspaceID),
		AccountID:            cloneUUIDPointer(scope.AccountID),
		ExpiresAt:            now.Add(c.ttl),
		LastAccessedAt:       now,
	}
	cached.Response.ParseSessionID = sessionID
	cached.SizeBytes = estimatePlaygroundCachedExecutionBytes(cached)
	if c.maxItemBytes > 0 && cached.SizeBytes > c.maxItemBytes {
		return ""
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	c.items[sessionID] = cached
	c.totalBytes += cached.SizeBytes
	c.pruneCapacityLocked()
	if _, ok := c.items[sessionID]; !ok {
		return ""
	}
	return sessionID
}

func (c *playgroundParseSessionCache) Get(sessionID string, scope service.PlaygroundRunListFilter) (*playgroundExecution, bool) {
	if c == nil {
		return nil, false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.items[sessionID]
	if !ok {
		return nil, false
	}
	if !cached.ExpiresAt.After(now) {
		c.deleteLocked(sessionID)
		return nil, false
	}
	if !samePlaygroundScope(cached.WorkspaceID, scope.WorkspaceID) || !samePlaygroundScope(cached.AccountID, scope.AccountID) {
		return nil, false
	}
	cached.LastAccessedAt = now
	c.items[sessionID] = cached
	return &playgroundExecution{
		Response:             cached.Response,
		RequestedProviderKey: cached.RequestedProviderKey,
		AdapterName:          cached.AdapterName,
		EffectiveRequest:     cached.EffectiveRequest,
		SourceMimeType:       cached.SourceMimeType,
		SourceFileExt:        cached.SourceFileExt,
	}, true
}

func (c *playgroundParseSessionCache) Delete(sessionID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteLocked(strings.TrimSpace(sessionID))
}

func (c *playgroundParseSessionCache) pruneExpiredLocked(now time.Time) {
	for key, item := range c.items {
		if !item.ExpiresAt.After(now) {
			c.deleteLocked(key)
		}
	}
}

func (c *playgroundParseSessionCache) pruneCapacityLocked() {
	for c.overCapacityLocked() {
		oldestKey := ""
		var oldestAt time.Time
		for key, item := range c.items {
			if oldestKey == "" || item.LastAccessedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = item.LastAccessedAt
			}
		}
		if oldestKey == "" {
			return
		}
		c.deleteLocked(oldestKey)
	}
}

func (c *playgroundParseSessionCache) overCapacityLocked() bool {
	if c.maxEntries > 0 && len(c.items) > c.maxEntries {
		return true
	}
	return c.maxBytes > 0 && c.totalBytes > c.maxBytes
}

func (c *playgroundParseSessionCache) deleteLocked(sessionID string) {
	if item, ok := c.items[sessionID]; ok {
		c.totalBytes -= item.SizeBytes
		if c.totalBytes < 0 {
			c.totalBytes = 0
		}
		delete(c.items, sessionID)
	}
}

func estimatePlaygroundCachedExecutionBytes(cached playgroundCachedExecution) int64 {
	data, err := json.Marshal(cached)
	if err != nil {
		return 1 << 20
	}
	return int64(len(data))
}

func samePlaygroundScope(left, right *uuid.UUID) bool {
	if left == nil || *left == uuid.Nil {
		return right == nil || *right == uuid.Nil
	}
	if right == nil {
		return false
	}
	return *left == *right
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func newPlaygroundParseSessionID() string {
	buf := make([]byte, 18)
	if _, err := cryptorand.Read(buf); err != nil {
		return uuid.NewString()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
