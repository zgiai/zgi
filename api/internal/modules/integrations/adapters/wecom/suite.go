package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const wecomInstallURL = "https://open.work.weixin.qq.com/3rdapp/install"

type SuiteConfig struct {
	SuiteID            string
	SuiteSecret        string
	CallbackToken      string
	EncodingAESKey     string
	InstallCallbackURL string
}

func (config SuiteConfig) Configured() bool {
	return strings.TrimSpace(config.SuiteID) != "" && strings.TrimSpace(config.SuiteSecret) != "" &&
		strings.TrimSpace(config.CallbackToken) != "" && strings.TrimSpace(config.EncodingAESKey) != "" &&
		strings.TrimSpace(config.InstallCallbackURL) != ""
}

type IntegrationProviderSuiteState struct {
	IntegrationID     string    `gorm:"size:64;primaryKey"`
	SuiteID           string    `gorm:"size:128;not null"`
	EncryptedTicket   string    `gorm:"type:text;not null"`
	CredentialVersion int       `gorm:"not null;default:1"`
	ReceivedAt        time.Time `gorm:"not null"`
	ExpiresAt         time.Time `gorm:"not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (IntegrationProviderSuiteState) TableName() string {
	return "integration_provider_suite_states"
}

type SuiteStateRepository struct {
	db     *gorm.DB
	cipher integrations.CredentialCipher
}

func NewSuiteStateRepository(db *gorm.DB, cipher integrations.CredentialCipher) *SuiteStateRepository {
	return &SuiteStateRepository{db: db, cipher: cipher}
}

func (repository *SuiteStateRepository) SaveTicket(ctx context.Context, suiteID, ticket string, receivedAt time.Time) error {
	if repository == nil || repository.db == nil || repository.cipher == nil {
		return fmt.Errorf("WeCom suite state repository is unavailable")
	}
	suiteID, ticket = strings.TrimSpace(suiteID), strings.TrimSpace(ticket)
	if suiteID == "" || ticket == "" || len(ticket) > 512 {
		return wecomError(integrations.ErrorCodeResponseInvalid, "WeCom suite ticket is invalid", nil)
	}
	receivedAt = receivedAt.UTC()
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	envelope, err := repository.cipher.EncryptCredentials(map[string]string{"suite_ticket": ticket}, suiteStateAAD())
	if err != nil {
		return fmt.Errorf("encrypt WeCom suite ticket: %w", err)
	}
	state := &IntegrationProviderSuiteState{
		IntegrationID: IntegrationID, SuiteID: suiteID, EncryptedTicket: envelope,
		CredentialVersion: 1, ReceivedAt: receivedAt, ExpiresAt: receivedAt.Add(30 * time.Minute),
	}
	return repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "integration_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"suite_id": suiteID, "encrypted_ticket": envelope, "credential_version": 1,
			"received_at": state.ReceivedAt, "expires_at": state.ExpiresAt,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(state).Error
}

func (repository *SuiteStateRepository) LoadTicket(ctx context.Context, suiteID string, now time.Time) (string, error) {
	if repository == nil || repository.db == nil || repository.cipher == nil {
		return "", fmt.Errorf("WeCom suite state repository is unavailable")
	}
	var state IntegrationProviderSuiteState
	err := repository.db.WithContext(ctx).Where("integration_id = ? AND suite_id = ?", IntegrationID, strings.TrimSpace(suiteID)).First(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", wecomError(integrations.ErrorCodeReconnectRequired, "WeCom suite ticket has not been received", err)
		}
		return "", fmt.Errorf("load WeCom suite ticket: %w", err)
	}
	if !state.ExpiresAt.After(now.UTC()) {
		return "", wecomError(integrations.ErrorCodeReconnectRequired, "WeCom suite ticket has expired", nil)
	}
	credentials, err := repository.cipher.DecryptCredentials(state.EncryptedTicket, suiteStateAAD())
	if err != nil {
		return "", wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom suite ticket could not be decrypted", err)
	}
	ticket := strings.TrimSpace(credentials["suite_ticket"])
	for key := range credentials {
		credentials[key] = ""
		delete(credentials, key)
	}
	if ticket == "" {
		return "", wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom suite ticket is unavailable", nil)
	}
	return ticket, nil
}

func suiteStateAAD() integrations.CredentialAAD {
	return integrations.CredentialAAD{
		OrganizationID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("zgi/provider-suite-state/organization/wecom")),
		ConnectionID:   uuid.NewSHA1(uuid.NameSpaceOID, []byte("zgi/provider-suite-state/connection/wecom")),
		IntegrationID:  "wecom-suite-state", CredentialVersion: 1,
	}
}

func (adapter *Adapter) BuildProviderInstallAuthorizationURL(ctx context.Context, request integrations.ProviderInstallAuthorizationRequest) (string, error) {
	if adapter == nil || !adapter.suite.Configured() || adapter.suiteState == nil {
		return "", wecomError(integrations.ErrorCodeDisabled, "WeCom third-party application is not configured", nil)
	}
	redirectURI := strings.TrimSpace(request.RedirectURI)
	if redirectURI == "" || redirectURI != strings.TrimSpace(adapter.suite.InstallCallbackURL) {
		return "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom install callback URL is invalid", nil)
	}
	state := strings.TrimSpace(request.State)
	if state == "" || len(state) > 128 {
		return "", wecomError(integrations.ErrorCodeInvalidInput, "WeCom install state is invalid", nil)
	}
	suiteToken, err := adapter.suiteAccessToken(ctx)
	if err != nil {
		return "", err
	}
	var response struct {
		apiEnvelope
		PreAuthCode string `json:"pre_auth_code"`
		ExpiresIn   int    `json:"expires_in"`
	}
	status, envelope, err := adapter.doJSON(ctx, http.MethodGet, "/cgi-bin/service/get_pre_auth_code", url.Values{"suite_access_token": []string{suiteToken}}, nil, &response)
	if err != nil {
		return "", err
	}
	if envelope.ErrCode != 0 {
		return "", mapProviderError(status, envelope)
	}
	if strings.TrimSpace(response.PreAuthCode) == "" {
		return "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom pre-authorization response is incomplete", nil)
	}
	target, _ := url.Parse(wecomInstallURL)
	query := target.Query()
	query.Set("suite_id", strings.TrimSpace(adapter.suite.SuiteID))
	query.Set("pre_auth_code", strings.TrimSpace(response.PreAuthCode))
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (adapter *Adapter) ExchangeProviderInstallCode(ctx context.Context, request integrations.ProviderInstallExchangeRequest) (integrations.ProviderInstallGrant, error) {
	grant := integrations.ProviderInstallGrant{}
	if adapter == nil || !adapter.suite.Configured() || adapter.suiteState == nil {
		return grant, wecomError(integrations.ErrorCodeDisabled, "WeCom third-party application is not configured", nil)
	}
	authCode := strings.TrimSpace(request.AuthorizationCode)
	if authCode == "" || len(authCode) > 512 {
		return grant, wecomError(integrations.ErrorCodeInvalidInput, "WeCom authorization code is invalid", nil)
	}
	suiteToken, err := adapter.suiteAccessToken(ctx)
	if err != nil {
		return grant, err
	}
	var permanent struct {
		apiEnvelope
		PermanentCode string `json:"permanent_code"`
		AuthCorpInfo  struct {
			CorpID   string `json:"corpid"`
			CorpName string `json:"corp_name"`
		} `json:"auth_corp_info"`
	}
	status, envelope, err := adapter.doJSON(ctx, http.MethodPost, "/cgi-bin/service/v2/get_permanent_code", url.Values{"suite_access_token": []string{suiteToken}}, map[string]string{"auth_code": authCode}, &permanent)
	if err != nil {
		return grant, err
	}
	if envelope.ErrCode != 0 {
		return grant, mapProviderError(status, envelope)
	}
	authCorpID, permanentCode := strings.TrimSpace(permanent.AuthCorpInfo.CorpID), strings.TrimSpace(permanent.PermanentCode)
	if authCorpID == "" || permanentCode == "" {
		return grant, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom permanent authorization response is incomplete", nil)
	}
	var authInfo struct {
		apiEnvelope
		AuthCorpInfo struct {
			CorpID   string `json:"corpid"`
			CorpName string `json:"corp_name"`
		} `json:"auth_corp_info"`
		AuthInfo struct {
			Agents []struct {
				AgentID int64  `json:"agentid"`
				Name    string `json:"name"`
			} `json:"agent"`
		} `json:"auth_info"`
	}
	status, envelope, err = adapter.doJSON(ctx, http.MethodPost, "/cgi-bin/service/v2/get_auth_info", url.Values{"suite_access_token": []string{suiteToken}}, map[string]string{"auth_corpid": authCorpID, "permanent_code": permanentCode}, &authInfo)
	if err != nil {
		return grant, err
	}
	if envelope.ErrCode != 0 {
		return grant, mapProviderError(status, envelope)
	}
	if len(authInfo.AuthInfo.Agents) != 1 || authInfo.AuthInfo.Agents[0].AgentID <= 0 {
		return grant, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom authorized application identity is unavailable", nil)
	}
	displayName := firstNonEmpty(authInfo.AuthCorpInfo.CorpName, permanent.AuthCorpInfo.CorpName, authInfo.AuthInfo.Agents[0].Name, "WeCom organization")
	return integrations.ProviderInstallGrant{
		ExternalTenantHash: adapter.externalTenantHash(authCorpID), AccountDisplayName: displayName,
		Credentials: map[string]string{
			"auth_corpid": authCorpID, "permanent_code": permanentCode,
			"agent_id": strconv.FormatInt(authInfo.AuthInfo.Agents[0].AgentID, 10),
		},
		Config:        map[string]any{"authorization_mode": "suite"},
		GrantedScopes: []string{ScopeApp, ScopeContacts, ScopeSend},
	}, nil
}

func (adapter *Adapter) suiteAccessToken(ctx context.Context) (string, error) {
	if adapter == nil || !adapter.suite.Configured() || adapter.suiteState == nil {
		return "", wecomError(integrations.ErrorCodeDisabled, "WeCom third-party application is not configured", nil)
	}
	adapter.mu.Lock()
	cached := adapter.suiteToken
	adapter.mu.Unlock()
	if cached.value != "" && time.Now().Before(cached.expiresAt) {
		return cached.value, nil
	}
	ticket, err := adapter.suiteState.LoadTicket(ctx, adapter.suite.SuiteID, time.Now())
	if err != nil {
		return "", err
	}
	var response struct {
		apiEnvelope
		SuiteAccessToken string `json:"suite_access_token"`
		ExpiresIn        int    `json:"expires_in"`
	}
	status, envelope, err := adapter.doJSON(ctx, http.MethodPost, "/cgi-bin/service/get_suite_token", nil, map[string]string{
		"suite_id": adapter.suite.SuiteID, "suite_secret": adapter.suite.SuiteSecret, "suite_ticket": ticket,
	}, &response)
	if err != nil {
		return "", err
	}
	if envelope.ErrCode != 0 {
		return "", mapProviderError(status, envelope)
	}
	if strings.TrimSpace(response.SuiteAccessToken) == "" {
		return "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom suite token response is incomplete", nil)
	}
	ttl := time.Duration(response.ExpiresIn)*time.Second - 5*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	adapter.mu.Lock()
	adapter.suiteToken = cachedToken{value: strings.TrimSpace(response.SuiteAccessToken), expiresAt: time.Now().Add(ttl)}
	adapter.mu.Unlock()
	return strings.TrimSpace(response.SuiteAccessToken), nil
}

func (adapter *Adapter) externalTenantHash(authCorpID string) string {
	digest := sha256.Sum256([]byte("zgi/wecom/external-tenant/v1\x00" + strings.TrimSpace(adapter.suite.SuiteID) + "\x00" + strings.TrimSpace(authCorpID)))
	return hex.EncodeToString(digest[:])
}

func (adapter *Adapter) ProviderTenantHash(authCorpID string) string {
	return adapter.externalTenantHash(authCorpID)
}
