package channel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultCacheTTL    = 30 * time.Second
	defaultHTTPTimeout = 5 * time.Second
	pathChannelRouting = "/v1/internal/channels/routing"
)

// Cloud is the cloud implementation that fetches channels via HTTP + local TTL cache.
// Replaces the previous gRPC streaming approach for simplicity and reliability.
type Cloud struct {
	baseURL        string
	internalAPIKey string
	httpClient     *http.Client

	cache     []*OfficialChannel
	cacheLock sync.RWMutex
	fetchedAt time.Time
	ttl       time.Duration
}

// NewCloud creates a new cloud channel provider with HTTP + TTL cache.
func NewCloud(baseURL string, internalAPIKey string) *Cloud {
	return &Cloud{
		baseURL:        baseURL,
		internalAPIKey: internalAPIKey,
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: defaultHTTPTimeout,
		}),
		ttl: defaultCacheTTL,
	}
}

// ListChannels returns the cached list of official channels for routing decisions.
// Uses lazy-loading: fetches from console-api on first call and when TTL expires.
// Falls back to stale cache if the HTTP request fails.
func (c *Cloud) ListChannels(ctx context.Context, tenantID string) ([]*OfficialChannel, error) {
	c.cacheLock.RLock()
	if time.Since(c.fetchedAt) < c.ttl && c.cache != nil {
		defer c.cacheLock.RUnlock()
		return c.deepCopy(), nil
	}
	c.cacheLock.RUnlock()

	return c.refresh(ctx)
}

// refresh fetches fresh channel data from console-api and updates the cache.
// On failure, returns stale cache data if available.
func (c *Cloud) refresh(ctx context.Context) ([]*OfficialChannel, error) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	// Double-check: another goroutine may have refreshed while we waited for the lock
	if time.Since(c.fetchedAt) < c.ttl && c.cache != nil {
		return c.deepCopy(), nil
	}

	channels, err := c.fetchFromConsole(ctx)
	if err != nil {
		// Fallback to stale cache if available
		if c.cache != nil {
			logger.WarnContext(ctx, "Platform channel refresh failed, using stale cache",
				zap.Int("channel_count", len(c.cache)),
				zap.Duration("cache_age", time.Since(c.fetchedAt).Round(time.Second)),
				zap.Error(err),
			)
			return c.deepCopy(), nil
		}
		return nil, fmt.Errorf("failed to fetch platform channels: %w", err)
	}

	c.cache = channels
	c.fetchedAt = time.Now()
	logger.InfoContext(ctx, "Platform channel cache refreshed", zap.Int("channel_count", len(channels)))

	return c.deepCopy(), nil
}

// fetchFromConsole calls console-api GET /v1/internal/channels/routing
func (c *Cloud) fetchFromConsole(ctx context.Context) ([]*OfficialChannel, error) {
	url := c.baseURL + pathChannelRouting
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("console-api unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("console-api returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var consoleResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Channels []struct {
				ID       string   `json:"id"`
				Provider string   `json:"provider"`
				Models   []string `json:"models"`
				Priority int      `json:"priority"`
				Weight   int      `json:"weight"`
			} `json:"channels"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &consoleResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if consoleResp.Code != 0 {
		return nil, fmt.Errorf("console-api error (code %d): %s", consoleResp.Code, consoleResp.Message)
	}

	channels := make([]*OfficialChannel, 0, len(consoleResp.Data.Channels))
	for _, ch := range consoleResp.Data.Channels {
		channels = append(channels, &OfficialChannel{
			ID:         ch.ID,
			Provider:   ch.Provider,
			Models:     ch.Models,
			Priority:   ch.Priority,
			Weight:     ch.Weight,
			APIBaseURL: c.baseURL + "/v1/internal", // console-api internal endpoint acts as OpenAI-compatible base
		})
	}

	return channels, nil
}

// signRequest adds HMAC-SHA256 authentication headers to an outgoing request.
// It sets X-Internal-Timestamp and X-Internal-Signature headers.
func (c *Cloud) signRequest(req *http.Request) {
	if c.internalAPIKey == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	message := ts + "|" + req.URL.Path
	mac := hmac.New(sha256.New, []byte(c.internalAPIKey))
	mac.Write([]byte(message))
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Internal-Timestamp", ts)
	req.Header.Set("X-Internal-Signature", sig)
}

// deepCopy returns a deep copy of the cached channels to prevent callers from mutating cache.
func (c *Cloud) deepCopy() []*OfficialChannel {
	result := make([]*OfficialChannel, len(c.cache))
	for i, ch := range c.cache {
		copied := *ch
		copied.Models = make([]string, len(ch.Models))
		copy(copied.Models, ch.Models)
		result[i] = &copied
	}
	return result
}
