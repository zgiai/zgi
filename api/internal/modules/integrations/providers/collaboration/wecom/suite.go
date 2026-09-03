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
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	wecomInstallURL       = "https://open.work.weixin.qq.com/3rdapp/install"
	installAuthModeFormal = "formal"
	installAuthModeTest   = "test"
)

type SuiteConfig struct {
	SuiteID                    string
	ProviderCorpID             string
	ProviderSecret             string
	SuiteSecret                string
	CallbackToken              string
	EncodingAESKey             string
	InstallAuthMode            string
	InstallCallbackURL         string
	CustomerContactEnabled     bool
	CustomerMassMessageEnabled bool
}

func (config SuiteConfig) Configured() bool {
	return strings.TrimSpace(config.SuiteID) != "" && strings.TrimSpace(config.ProviderCorpID) != "" &&
		strings.TrimSpace(config.ProviderSecret) != "" &&
		strings.TrimSpace(config.SuiteSecret) != "" &&
		strings.TrimSpace(config.CallbackToken) != "" && strings.TrimSpace(config.EncodingAESKey) != "" &&
		strings.TrimSpace(config.InstallCallbackURL) != ""
}

func (config SuiteConfig) installAuthType() (int, error) {
	switch strings.ToLower(strings.TrimSpace(config.InstallAuthMode)) {
	case "", installAuthModeFormal:
		return 0, nil
	case installAuthModeTest:
		return 1, nil
	default:
		return 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom install authorization mode is invalid", nil)
	}
}

type installSessionInfoRequest struct {
	PreAuthCode string             `json:"pre_auth_code"`
	SessionInfo installSessionInfo `json:"session_info"`
}

type installSessionInfo struct {
	AuthType int `json:"auth_type"`
}

const (
	wecomAuthorizationModeAdministrator = 0
	wecomAuthorizationModeMember        = 1
)

type authorizationPrivilege struct {
	Level      *int     `json:"level"`
	AllowParty []int64  `json:"allow_party"`
	AllowUser  []string `json:"allow_user"`
	AllowTag   []int64  `json:"allow_tag"`
	ExtraParty []int64  `json:"extra_party"`
	ExtraUser  []string `json:"extra_user"`
	ExtraTag   []int64  `json:"extra_tag"`
}

type authorizationAgent struct {
	AgentID         int64                   `json:"agentid"`
	Name            string                  `json:"name"`
	AuthMode        *int                    `json:"auth_mode"`
	Privilege       *authorizationPrivilege `json:"privilege"`
	IsCustomizedApp bool                    `json:"is_customized_app"`
}

type authorizationInfoResponse struct {
	apiEnvelope
	AuthCorpInfo struct {
		CorpID   string `json:"corpid"`
		CorpName string `json:"corp_name"`
	} `json:"auth_corp_info"`
	AuthInfo struct {
		Agents []authorizationAgent `json:"agent"`
	} `json:"auth_info"`
}

func (privilege *authorizationPrivilege) effectiveParties() []int64 {
	if privilege == nil {
		return nil
	}
	return mergePositiveInt64(privilege.AllowParty, privilege.ExtraParty)
}

func (privilege *authorizationPrivilege) effectiveUsers() []string {
	if privilege == nil {
		return nil
	}
	return mergeAuthorizationUsers(privilege.AllowUser, privilege.ExtraUser)
}

func (privilege *authorizationPrivilege) effectiveTags() []int64 {
	if privilege == nil {
		return nil
	}
	return mergePositiveInt64(privilege.AllowTag, privilege.ExtraTag)
}

func mergePositiveInt64(groups ...[]int64) []int64 {
	seen := make(map[int64]struct{})
	values := make([]int64, 0)
	for _, group := range groups {
		for _, value := range group {
			if value <= 0 {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values
}

func mergeAuthorizationUsers(groups ...[]string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, group := range groups {
		for _, raw := range group {
			value := strings.TrimSpace(raw)
			if value == "" || len(value) > 64 || strings.ContainsAny(value, "|\r\n") {
				continue
			}
			key := strings.ToLower(value)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			values = append(values, value)
		}
	}
	return values
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
	preAuthCode := strings.TrimSpace(response.PreAuthCode)
	if preAuthCode == "" {
		return "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom pre-authorization response is incomplete", nil)
	}
	installAuthType, err := adapter.suite.installAuthType()
	if err != nil {
		return "", err
	}
	status, envelope, err = adapter.doJSON(
		ctx,
		http.MethodPost,
		"/cgi-bin/service/set_session_info",
		url.Values{"suite_access_token": []string{suiteToken}},
		installSessionInfoRequest{
			PreAuthCode: preAuthCode,
			SessionInfo: installSessionInfo{AuthType: installAuthType},
		},
		nil,
	)
	if err != nil {
		return "", err
	}
	if envelope.ErrCode != 0 {
		return "", mapProviderError(status, envelope)
	}
	target, _ := url.Parse(wecomInstallURL)
	query := target.Query()
	query.Set("suite_id", strings.TrimSpace(adapter.suite.SuiteID))
	query.Set("pre_auth_code", preAuthCode)
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
	authInfo, authorizedAgent, err := adapter.getAuthorizationInfo(ctx, authCorpID, permanentCode, 0)
	if err != nil {
		return grant, err
	}
	displayName := firstNonEmpty(authInfo.AuthCorpInfo.CorpName, permanent.AuthCorpInfo.CorpName, authorizedAgent.Name, "WeCom organization")
	return integrations.ProviderInstallGrant{
		ExternalTenantHash: adapter.externalTenantHash(authCorpID), AccountDisplayName: displayName,
		Credentials: map[string]string{
			"auth_corpid": authCorpID, "permanent_code": permanentCode,
			"agent_id": strconv.FormatInt(authorizedAgent.AgentID, 10),
		},
		// Authorization mode and visible subjects can change through change_auth.
		// Keep them out of persisted public configuration and read get_auth_info at
		// execution time instead.
		Config:        map[string]any{"authorization_mode": "suite"},
		GrantedScopes: declaredGrantedScopes(adapter.suite),
	}, nil
}

func (adapter *Adapter) getAuthorizationInfo(ctx context.Context, authCorpID, permanentCode string, expectedAgentID int64) (authorizationInfoResponse, authorizationAgent, error) {
	response := authorizationInfoResponse{}
	authCorpID, permanentCode = strings.TrimSpace(authCorpID), strings.TrimSpace(permanentCode)
	if authCorpID == "" || permanentCode == "" {
		return response, authorizationAgent{}, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom authorization credentials are unavailable", nil)
	}
	suiteToken, err := adapter.suiteAccessToken(ctx)
	if err != nil {
		return response, authorizationAgent{}, err
	}
	status, envelope, err := adapter.doJSON(ctx, http.MethodPost, "/cgi-bin/service/v2/get_auth_info", url.Values{"suite_access_token": []string{suiteToken}}, map[string]string{"auth_corpid": authCorpID, "permanent_code": permanentCode}, &response)
	if err != nil {
		return response, authorizationAgent{}, err
	}
	if envelope.ErrCode != 0 {
		return response, authorizationAgent{}, mapProviderError(status, envelope)
	}
	if corpID := strings.TrimSpace(response.AuthCorpInfo.CorpID); corpID == "" || corpID != authCorpID {
		return response, authorizationAgent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom authorization tenant identity is invalid", nil)
	}
	if expectedAgentID == 0 {
		if len(response.AuthInfo.Agents) != 1 {
			return response, authorizationAgent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom authorized application identity is unavailable", nil)
		}
		validated, validateErr := validateAuthorizationAgent(response.AuthInfo.Agents[0])
		return response, validated, validateErr
	}
	for _, agent := range response.AuthInfo.Agents {
		if agent.AgentID == expectedAgentID {
			validated, validateErr := validateAuthorizationAgent(agent)
			return response, validated, validateErr
		}
	}
	return response, authorizationAgent{}, wecomError(integrations.ErrorCodeInsufficientScope, "WeCom authorized application identity is unavailable", nil)
}

func validateAuthorizationAgent(agent authorizationAgent) (authorizationAgent, error) {
	if agent.AgentID <= 0 || agent.AuthMode == nil || agent.Privilege == nil || agent.Privilege.Level == nil {
		return authorizationAgent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom authorization application details are incomplete", nil)
	}
	if *agent.AuthMode != wecomAuthorizationModeAdministrator && *agent.AuthMode != wecomAuthorizationModeMember {
		return authorizationAgent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom authorization mode is invalid", nil)
	}
	level := *agent.Privilege.Level
	if agent.IsCustomizedApp {
		if level != 0 {
			return authorizationAgent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom customized application privilege is invalid", nil)
		}
		return agent, nil
	}
	if level < 1 || level > 5 {
		return authorizationAgent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom application privilege level is invalid", nil)
	}
	return agent, nil
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
