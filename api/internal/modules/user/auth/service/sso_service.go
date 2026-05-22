package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"gorm.io/gorm"
)

const (
	TokenTypeSSOState       = "sso_state"
	TokenTypeSSOLoginTicket = "sso_login_ticket"

	ssoStateFrontendCallbackURLKey = "frontend_callback_url"
	ssoLoginTicketIssuedAtUnixKey  = "issued_at_unix"
	ssoLoginTicketProviderKey      = "provider"
	ssoLoginTicketIDTokenKey       = "id_token"
	ssoLoginTicketMaxConsumeCount  = 2
	ssoLoginTicketGraceWindow      = 3 * time.Minute
)

var (
	errSSOTokenManagerRequired = errors.New("token manager is required")
	errSSOAccountRequired      = errors.New("account is required")
	errSSOLoginTicketAccountID = errors.New("account id missing from login ticket")
	errSSOLoginTicketExpired   = errors.New("sso login ticket expired")
	errSSOLoginTicketConsumed  = errors.New("sso login ticket consumed too many times")
)

func (s *AccountService) IssueSSOState(ctx context.Context, callbackURL string) (string, error) {
	if s.tokenMgr == nil {
		return "", errSSOTokenManagerRequired
	}

	emptyEmail := ""
	var extra map[string]any
	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL != "" {
		extra = map[string]any{ssoStateFrontendCallbackURLKey: callbackURL}
	}
	return s.tokenMgr.GenerateToken(ctx, TokenTypeSSOState, nil, &emptyEmail, extra)
}

func (s *AccountService) ConsumeSSOState(ctx context.Context, state string) (string, error) {
	if s.tokenMgr == nil {
		return "", errSSOTokenManagerRequired
	}
	tokenData, err := s.tokenMgr.GetTokenData(state, TokenTypeSSOState)
	if err != nil {
		return "", err
	}
	callbackURL := ssoStateCallbackURLFromExtra(tokenData.Extra)
	if err := s.tokenMgr.RevokeToken(state, TokenTypeSSOState); err != nil {
		return "", err
	}
	return callbackURL, nil
}

func (s *AccountService) ResolveOrCreateSSOAccount(ctx context.Context, identity *shared_dto.SSOIdentity) (*auth_model.Account, error) {
	if identity == nil {
		return nil, errors.New("identity is required")
	}
	if strings.TrimSpace(identity.Subject) == "" {
		return nil, errors.New("subject is required")
	}

	if account, err := s.findAccountByOIDCSubject(ctx, identity.Subject); err != nil {
		return nil, err
	} else if account != nil {
		return s.syncSSOAccount(ctx, account, identity)
	}

	emailAccount, err := s.findAccountByEmail(ctx, identity.Email)
	if err != nil {
		return nil, err
	}
	mobileAccount, err := s.findAccountByMobile(ctx, identity.PhoneNumber, identity.CountryCode)
	if err != nil {
		return nil, err
	}

	account := selectPreferredSSOAccount(emailAccount, mobileAccount)
	if account != nil {
		account, err = s.syncSSOAccount(ctx, account, identity)
		if err != nil {
			return nil, err
		}
		if err := s.LinkAccountIntegrate(ctx, auth_model.ProviderOIDC, identity.Subject, account); err != nil {
			return nil, err
		}
		return account, nil
	}

	account, err = s.registerSSOAccount(ctx, identity)
	if err != nil {
		return nil, err
	}
	if err := s.LinkAccountIntegrate(ctx, auth_model.ProviderOIDC, identity.Subject, account); err != nil {
		return nil, err
	}
	return account, nil
}

func (s *AccountService) IssueSSOLoginTicket(ctx context.Context, account *auth_model.Account, sso *shared_dto.SSOProviderToken) (string, error) {
	if s.tokenMgr == nil {
		return "", errSSOTokenManagerRequired
	}
	if account == nil {
		return "", errSSOAccountRequired
	}

	extra := map[string]any{
		ssoLoginTicketIssuedAtUnixKey: time.Now().UTC().Unix(),
	}
	if sso != nil {
		if provider := strings.TrimSpace(sso.Provider); provider != "" {
			extra[ssoLoginTicketProviderKey] = provider
		}
		if idToken := strings.TrimSpace(sso.IDToken); idToken != "" {
			extra[ssoLoginTicketIDTokenKey] = idToken
		}
	}

	return s.tokenMgr.GenerateToken(ctx, TokenTypeSSOLoginTicket, account, nil, extra)
}

func (s *AccountService) ConsumeSSOLoginTicket(ctx context.Context, ticket, ipAddress string) (*shared_dto.LoginResponse, error) {
	if s.tokenMgr == nil {
		return nil, errSSOTokenManagerRequired
	}

	tokenData, err := s.tokenMgr.GetTokenData(ticket, TokenTypeSSOLoginTicket)
	if err != nil {
		return nil, err
	}
	if tokenData == nil || tokenData.AccountID == nil {
		return nil, errSSOLoginTicketAccountID
	}

	remainingWindow, legacyTicket, err := ssoLoginTicketRemainingWindow(tokenData.Extra, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if legacyTicket {
		if err := s.tokenMgr.RevokeToken(ticket, TokenTypeSSOLoginTicket); err != nil {
			return nil, err
		}
	} else if remainingWindow <= 0 {
		_ = s.tokenMgr.RevokeToken(ticket, TokenTypeSSOLoginTicket)
		return nil, errSSOLoginTicketExpired
	}

	usageCount := int64(0)
	if !legacyTicket {
		usageCount, err = s.tokenMgr.IncrementTokenUsage(ctx, ticket, TokenTypeSSOLoginTicket, remainingWindow)
		if err != nil {
			return nil, err
		}
		if usageCount > ssoLoginTicketMaxConsumeCount {
			_ = s.tokenMgr.RevokeToken(ticket, TokenTypeSSOLoginTicket)
			return nil, errSSOLoginTicketConsumed
		}
	}

	account, err := s.accountRepo.GetAccount(ctx, *tokenData.AccountID)
	if err != nil {
		if !legacyTicket {
			return nil, s.rollbackSSOLoginTicketUsage(ctx, ticket, err)
		}
		return nil, err
	}

	tokenPair, err := s.LoginCommon(account, ipAddress)
	if err != nil {
		if !legacyTicket {
			return nil, s.rollbackSSOLoginTicketUsage(ctx, ticket, err)
		}
		return nil, err
	}
	profile, err := s.GetAccountProfile(ctx, account.ID)
	if err != nil {
		if !legacyTicket {
			return nil, s.rollbackSSOLoginTicketUsage(ctx, ticket, err)
		}
		return nil, err
	}

	if !legacyTicket && usageCount >= ssoLoginTicketMaxConsumeCount {
		_ = s.tokenMgr.RevokeToken(ticket, TokenTypeSSOLoginTicket)
	}

	return &shared_dto.LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		Account:      profile,
		SSO:          ssoProviderTokenFromExtra(tokenData.Extra),
	}, nil
}

func ssoLoginTicketRemainingWindow(extra map[string]any, now time.Time) (time.Duration, bool, error) {
	issuedAt, ok, err := ssoLoginTicketIssuedAt(extra)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, true, nil
	}

	remaining := issuedAt.Add(ssoLoginTicketGraceWindow).Sub(now)
	if remaining <= 0 {
		return 0, false, nil
	}
	return remaining, false, nil
}

func ssoLoginTicketIssuedAt(extra map[string]any) (time.Time, bool, error) {
	if len(extra) == 0 {
		return time.Time{}, false, nil
	}

	rawIssuedAt, ok := extra[ssoLoginTicketIssuedAtUnixKey]
	if !ok || rawIssuedAt == nil {
		return time.Time{}, false, nil
	}

	issuedAtUnix, err := int64FromAny(rawIssuedAt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid sso login ticket issued at: %w", err)
	}

	return time.Unix(issuedAtUnix, 0).UTC(), true, nil
}

func int64FromAny(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}

func ssoProviderTokenFromExtra(extra map[string]any) *shared_dto.SSOProviderToken {
	if len(extra) == 0 {
		return nil
	}

	provider, _ := stringFromAny(extra[ssoLoginTicketProviderKey])
	idToken, _ := stringFromAny(extra[ssoLoginTicketIDTokenKey])
	if provider == "" && idToken == "" {
		return nil
	}

	return &shared_dto.SSOProviderToken{
		Provider: provider,
		IDToken:  idToken,
	}
}

func ssoStateCallbackURLFromExtra(extra map[string]any) string {
	callbackURL, _ := stringFromAny(extra[ssoStateFrontendCallbackURLKey])
	return callbackURL
}

func stringFromAny(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

func (s *AccountService) rollbackSSOLoginTicketUsage(ctx context.Context, ticket string, cause error) error {
	if _, rollbackErr := s.tokenMgr.DecrementTokenUsage(ctx, ticket, TokenTypeSSOLoginTicket); rollbackErr != nil {
		return errors.Join(cause, rollbackErr)
	}
	return cause
}

func (s *AccountService) findAccountByOIDCSubject(ctx context.Context, subject string) (*auth_model.Account, error) {
	integration, err := s.accountRepo.GetAccountIntegrateByProviderOpenID(ctx, auth_model.ProviderOIDC, subject)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	account, err := s.accountRepo.GetAccount(ctx, integration.AccountID)
	if err != nil {
		return nil, err
	}
	return account, nil
}

func (s *AccountService) findAccountByEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return nil, nil
	}

	account, err := s.accountRepo.GetAccountByEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return account, nil
}

func (s *AccountService) findAccountByMobile(ctx context.Context, phoneNumber, countryCode string) (*auth_model.Account, error) {
	normalizedMobile, err := normalizeSSOMobile(phoneNumber, countryCode)
	if err != nil {
		return nil, err
	}
	if normalizedMobile == "" {
		return nil, nil
	}

	account, err := s.accountRepo.GetAccountByNormalizedMobile(ctx, normalizedMobile)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return account, nil
}

func (s *AccountService) syncSSOAccount(ctx context.Context, account *auth_model.Account, identity *shared_dto.SSOIdentity) (*auth_model.Account, error) {
	if account == nil {
		return nil, errSSOAccountRequired
	}

	if err := ensureSSOAccountAllowed(account); err != nil {
		return nil, err
	}

	changed := false

	email := strings.ToLower(strings.TrimSpace(identity.Email))
	if account.Email == "" && email != "" {
		account.Email = email
		changed = true
	}

	normalizedMobile, err := normalizeSSOMobile(identity.PhoneNumber, identity.CountryCode)
	if err != nil {
		return nil, err
	}
	if account.MobileE164 == nil && normalizedMobile != "" {
		setAccountMobile(account, &normalizedMobile)
		changed = true
	}

	if strings.TrimSpace(account.Name) == "" {
		account.Name = defaultSSOAccountName(identity)
		changed = true
	}

	if account.Status == auth_model.AccountStatusPending || account.Status == auth_model.AccountStatusUninitialized {
		account.Status = auth_model.AccountStatusActive
		changed = true
	}

	if changed {
		if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
			return nil, err
		}
	}

	return account, nil
}

func (s *AccountService) registerSSOAccount(ctx context.Context, identity *shared_dto.SSOIdentity) (*auth_model.Account, error) {
	createWorkspaceRequired := true
	account, err := s.RegisterEx(
		ctx,
		strings.ToLower(strings.TrimSpace(identity.Email)),
		defaultSSOAccountName(identity),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&createWorkspaceRequired,
	)
	if err != nil {
		return nil, fmt.Errorf("register sso account: %w", err)
	}

	normalizedMobile, err := normalizeSSOMobile(identity.PhoneNumber, identity.CountryCode)
	if err != nil {
		return nil, err
	}
	if normalizedMobile == "" {
		return account, nil
	}

	if _, err := s.CreateAccountEx(ctx, account, normalizedMobile, nil); err != nil {
		return nil, err
	}

	return account, nil
}

func ensureSSOAccountAllowed(account *auth_model.Account) error {
	switch account.Status {
	case auth_model.AccountStatusBanned, auth_model.AccountStatusClosed, auth_model.AccountStatusFrozen:
		return fmt.Errorf("account status %s does not allow sso login", account.Status)
	default:
		return nil
	}
}
