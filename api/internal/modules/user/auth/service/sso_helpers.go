package service

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/nyaruka/phonenumbers"
	shared_dto "github.com/zgiai/ginext/internal/dto"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
)

var (
	ErrOIDCSubjectClaimRequired  = errors.New("oidc subject claim is required")
	ErrOIDCInvalidPhoneClaim     = errors.New("oidc phone claim is invalid")
	ErrOIDCIdentityContactAbsent = errors.New("oidc email or phone claim is required")
)

const (
	ssoRedirectTicketParam  = "ticket"
	ssoRedirectErrorParam   = "error"
	ssoRedirectReasonParam  = "reason"
	oidcResponseTypeKey     = "response_type"
	oidcResponseTypeCode    = "code"
	oidcClientIDKey         = "client_id"
	oidcRedirectURIKey      = "redirect_uri"
	oidcScopeKey            = "scope"
	oidcStateKey            = "state"
	oidcClaimAudience       = "aud"
	oidcClaimCountryCode    = "country_code"
	oidcClaimCountryCodeV2  = "countryCode"
	oidcClaimDisplayName    = "displayName"
	oidcClaimEmail          = "email"
	oidcClaimIssuer         = "iss"
	oidcClaimName           = "name"
	oidcClaimPhone          = "phone"
	oidcClaimPhoneNumber    = "phone_number"
	oidcClaimPreferredUser  = "preferred_username"
	oidcClaimSubject        = "sub"
	e164Prefix              = "+"
	ssoDefaultAccountName   = "user"
	ssoDefaultAccountPrefix = "user-"
	ssoPhoneSuffixLength    = 4
)

type casdoorOIDCConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

type oidcDiscoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	Issuer                string `json:"issuer"`
}

type oidcIdentity struct {
	Subject     string
	Email       string
	Name        string
	PhoneNumber string
	CountryCode string
}

func normalizeSSOMobile(phoneNumber, countryCode string) (string, error) {
	rawPhoneNumber := strings.TrimSpace(phoneNumber)
	rawCountryCode := strings.TrimSpace(countryCode)
	phoneNumber = normalizePhoneDigits(rawPhoneNumber, true)

	switch {
	case phoneNumber == "":
		return "", nil
	case strings.HasPrefix(phoneNumber, e164Prefix):
		rest, _ := strings.CutPrefix(phoneNumber, e164Prefix)
		if !isNumeric(rest) {
			return "", errors.New("phone number must contain digits after plus")
		}
		return phoneNumber, nil
	}

	phoneDigits := normalizePhoneDigits(rawPhoneNumber, false)
	switch {
	case phoneDigits == "":
		return "", nil
	case rawCountryCode == "":
		return "", errors.New("country code required for non-e164 phone number")
	case !isNumeric(phoneDigits):
		return "", errors.New("phone number must contain only digits")
	}

	if regionCode := normalizeISORegionCode(rawCountryCode); regionCode != "" {
		parsedNumber, err := phonenumbers.Parse(phoneDigits, regionCode)
		if err != nil {
			return "", fmt.Errorf("parse phone number for region %s: %w", regionCode, err)
		}
		if !phonenumbers.IsValidNumber(parsedNumber) {
			return "", fmt.Errorf("phone number is not valid for region %s", regionCode)
		}
		return phonenumbers.Format(parsedNumber, phonenumbers.E164), nil
	}

	countryDigits := normalizePhoneDigits(rawCountryCode, false)
	if !isNumeric(countryDigits) {
		return "", errors.New("country code must contain only digits or ISO region code")
	}

	return e164Prefix + countryDigits + phoneDigits, nil
}

func BuildFrontendSSORedirect(baseURL, ticket, errCode, reason string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse frontend redirect url: %w", err)
	}

	query := parsed.Query()
	if ticket != "" {
		query.Set(ssoRedirectTicketParam, ticket)
	}
	if errCode != "" {
		query.Set(ssoRedirectErrorParam, errCode)
	}
	if reason != "" {
		query.Set(ssoRedirectReasonParam, reason)
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func buildCasdoorAuthorizationURL(discovery oidcDiscoveryDocument, cfg casdoorOIDCConfig, state string) (string, error) {
	if discovery.AuthorizationEndpoint == "" {
		return "", errors.New("authorization endpoint is required")
	}
	if cfg.ClientID == "" {
		return "", errors.New("client id is required")
	}
	if cfg.RedirectURI == "" {
		return "", errors.New("redirect uri is required")
	}
	if state == "" {
		return "", errors.New("state is required")
	}

	parsed, err := url.Parse(discovery.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorization endpoint: %w", err)
	}

	query := parsed.Query()
	query.Set(oidcResponseTypeKey, oidcResponseTypeCode)
	query.Set(oidcClientIDKey, cfg.ClientID)
	query.Set(oidcRedirectURIKey, cfg.RedirectURI)
	query.Set(oidcScopeKey, strings.Join(cfg.Scopes, oidcScopeSeparator))
	query.Set(oidcStateKey, state)
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func identityFromClaims(claims map[string]any) (*oidcIdentity, error) {
	subject, _ := stringClaim(claims, oidcClaimSubject)
	if subject == "" {
		return nil, ErrOIDCSubjectClaimRequired
	}

	email, _ := stringClaim(claims, oidcClaimEmail)
	email = strings.ToLower(strings.TrimSpace(email))
	name, _ := firstNonEmptyClaim(claims, oidcClaimName, oidcClaimDisplayName, oidcClaimPreferredUser)
	phone, _ := firstNonEmptyClaim(claims, oidcClaimPhoneNumber, oidcClaimPhone)
	countryCode, _ := firstNonEmptyClaim(claims, oidcClaimCountryCode, oidcClaimCountryCodeV2)

	normalizedPhone, err := normalizeSSOMobile(phone, countryCode)
	if err != nil {
		if email == "" {
			return nil, fmt.Errorf("%w: %v", ErrOIDCInvalidPhoneClaim, err)
		}
		normalizedPhone = ""
	}
	if email == "" && normalizedPhone == "" {
		return nil, ErrOIDCIdentityContactAbsent
	}

	return &oidcIdentity{
		Subject:     subject,
		Email:       email,
		Name:        strings.TrimSpace(name),
		PhoneNumber: normalizedPhone,
		CountryCode: countryCode,
	}, nil
}

func selectPreferredSSOAccount(emailAccount, mobileAccount *auth_model.Account) *auth_model.Account {
	if emailAccount != nil {
		return emailAccount
	}
	return mobileAccount
}

func defaultSSOAccountName(identity *shared_dto.SSOIdentity) string {
	if identity == nil {
		return ssoDefaultAccountName
	}

	if name := strings.TrimSpace(identity.Name); name != "" {
		return name
	}
	if email := strings.TrimSpace(identity.Email); email != "" {
		localPart, _, found := strings.Cut(email, "@")
		if found && localPart != "" {
			return localPart
		}
		return email
	}
	if phone := strings.TrimSpace(identity.PhoneNumber); phone != "" {
		digits := normalizePhoneDigits(phone, false)
		if len(digits) >= ssoPhoneSuffixLength {
			return ssoDefaultAccountPrefix + digits[len(digits)-ssoPhoneSuffixLength:]
		}
		return ssoDefaultAccountPrefix + digits
	}
	return ssoDefaultAccountName
}

func firstNonEmptyClaim(claims map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := stringClaim(claims, key)
		if ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func stringClaim(claims map[string]any, key string) (string, bool) {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return "", false
	}

	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value), true
	case fmt.Stringer:
		return strings.TrimSpace(value.String()), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case int:
		return strconv.Itoa(value), true
	default:
		return "", false
	}
}

func normalizePhoneDigits(value string, keepLeadingPlus bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))

	for i, r := range value {
		if keepLeadingPlus && i == 0 && r == '+' {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	return b.String()
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeISORegionCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 {
		return ""
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return value
}
