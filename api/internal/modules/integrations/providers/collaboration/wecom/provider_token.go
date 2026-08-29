package wecom

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

const providerTokenRefreshSkew = 5 * time.Minute

func (adapter *Adapter) providerAccessToken(ctx context.Context) (string, error) {
	if adapter == nil {
		return "", wecomError(integrations.ErrorCodeDisabled, "WeCom service-provider credentials are not configured", nil)
	}
	providerCorpID := strings.TrimSpace(adapter.suite.ProviderCorpID)
	providerSecret := strings.TrimSpace(adapter.suite.ProviderSecret)
	if providerCorpID == "" || providerSecret == "" {
		return "", wecomError(integrations.ErrorCodeDisabled, "WeCom service-provider credentials are not configured", nil)
	}

	credentialHash := providerTokenCredentialHash(providerCorpID, providerSecret)
	adapter.mu.Lock()
	cached := adapter.providerToken
	adapter.mu.Unlock()
	if cached.value != "" && cached.credentialHash == credentialHash && time.Now().Before(cached.expiresAt) {
		return cached.value, nil
	}

	var response struct {
		apiEnvelope
		ProviderAccessToken string `json:"provider_access_token"`
		ExpiresIn           int    `json:"expires_in"`
	}
	status, envelope, err := adapter.doJSON(
		ctx,
		http.MethodPost,
		"/cgi-bin/service/get_provider_token",
		nil,
		map[string]string{
			"corpid":          providerCorpID,
			"provider_secret": providerSecret,
		},
		&response,
	)
	if err != nil {
		return "", err
	}
	if envelope.ErrCode != 0 {
		return "", mapProviderError(status, envelope)
	}

	token := strings.TrimSpace(response.ProviderAccessToken)
	if token == "" || response.ExpiresIn <= 0 {
		return "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom provider token response is incomplete", nil)
	}
	expiresAt := time.Now().Add(time.Duration(response.ExpiresIn)*time.Second - providerTokenRefreshSkew)
	adapter.mu.Lock()
	adapter.providerToken = cachedToken{value: token, expiresAt: expiresAt, credentialHash: credentialHash}
	adapter.mu.Unlock()
	return token, nil
}

func (adapter *Adapter) evictProviderAccessToken() {
	if adapter == nil {
		return
	}
	adapter.mu.Lock()
	adapter.providerToken = cachedToken{}
	adapter.mu.Unlock()
}

func providerTokenCredentialHash(providerCorpID, providerSecret string) [32]byte {
	return sha256.Sum256([]byte(
		"zgi/wecom/provider-token/v1\x00" + strings.TrimSpace(providerCorpID) + "\x00" + strings.TrimSpace(providerSecret),
	))
}
