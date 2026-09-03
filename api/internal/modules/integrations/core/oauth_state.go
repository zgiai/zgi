package integrations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OAuthStateStatus string

const (
	OAuthStatePending  OAuthStateStatus = "pending"
	OAuthStateConsumed OAuthStateStatus = "consumed"
)

type IntegrationOAuthState struct {
	ID                   uuid.UUID        `gorm:"type:uuid;primaryKey" json:"-"`
	StateDigest          string           `gorm:"size:64;not null;uniqueIndex" json:"-"`
	BrowserBindingDigest string           `gorm:"size:64;not null" json:"-"`
	OrganizationID       uuid.UUID        `gorm:"type:uuid;not null;index" json:"-"`
	AccountID            uuid.UUID        `gorm:"type:uuid;not null" json:"-"`
	FlowID               uuid.UUID        `gorm:"type:uuid;not null;index" json:"-"`
	ConnectionID         *uuid.UUID       `gorm:"type:uuid" json:"-"`
	IntegrationID        string           `gorm:"size:64;not null" json:"-"`
	DriverID             string           `gorm:"size:64;not null" json:"-"`
	AuthMethodID         string           `gorm:"size:128;not null" json:"-"`
	RedirectURI          string           `gorm:"size:2048;not null" json:"-"`
	RequestedScopes      []string         `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"-"`
	EncryptedVerifier    string           `gorm:"type:text;not null" json:"-"`
	Status               OAuthStateStatus `gorm:"size:32;not null" json:"-"`
	ExpiresAt            time.Time        `gorm:"not null;index" json:"-"`
	ConsumedAt           *time.Time       `json:"-"`
	CreatedAt            time.Time        `json:"-"`
}

func (IntegrationOAuthState) TableName() string { return "integration_oauth_states" }

type OAuthStateRepository interface {
	Create(ctx context.Context, state *IntegrationOAuthState) error
	Consume(ctx context.Context, stateDigest, browserBindingDigest string, now time.Time) (*IntegrationOAuthState, error)
}

type GormOAuthStateRepository struct{ db *gorm.DB }

func NewGormOAuthStateRepository(db *gorm.DB) *GormOAuthStateRepository {
	return &GormOAuthStateRepository{db: db}
}

func (repository *GormOAuthStateRepository) Create(ctx context.Context, state *IntegrationOAuthState) error {
	if repository == nil || repository.db == nil || state == nil {
		return fmt.Errorf("OAuth state repository is unavailable")
	}
	if err := repository.db.WithContext(ctx).Create(state).Error; err != nil {
		return fmt.Errorf("create integration OAuth state: %w", err)
	}
	return nil
}

func (repository *GormOAuthStateRepository) Consume(ctx context.Context, stateDigest, browserBindingDigest string, now time.Time) (*IntegrationOAuthState, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("OAuth state repository is unavailable")
	}
	var consumed IntegrationOAuthState
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state IntegrationOAuthState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("state_digest = ? AND browser_binding_digest = ?", strings.TrimSpace(stateDigest), strings.TrimSpace(browserBindingDigest)).
			First(&state).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return NewError(ErrorCodeAuthInvalid, "integration OAuth state is expired or already used", nil)
			}
			return fmt.Errorf("find integration OAuth state: %w", err)
		}
		if state.Status != OAuthStatePending || !state.ExpiresAt.After(now.UTC()) {
			return NewError(ErrorCodeAuthInvalid, "integration OAuth state is expired or already used", nil)
		}
		result := tx.Model(&IntegrationOAuthState{}).Where("id = ? AND status = ?", state.ID, OAuthStatePending).
			Updates(map[string]any{
				"status": OAuthStateConsumed, "consumed_at": now.UTC(),
				// The copy returned from this transaction still contains the
				// verifier for one decrypt attempt. The database copy is erased
				// immediately, including when that decrypt later fails.
				"encrypted_verifier": "",
			})
		if result.Error != nil {
			return fmt.Errorf("consume integration OAuth state: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return NewError(ErrorCodeAuthInvalid, "integration OAuth state is expired or already used", nil)
		}
		state.Status = OAuthStateConsumed
		state.ConsumedAt = cloneTimePointer(&now)
		consumed = state
		return nil
	})
	return &consumed, err
}

type OAuthStateCreateRequest struct {
	OrganizationID       uuid.UUID
	AccountID            uuid.UUID
	FlowID               uuid.UUID
	BrowserBindingDigest string
	ConnectionID         *uuid.UUID
	IntegrationID        string
	DriverID             string
	AuthMethodID         string
	RedirectURI          string
	RequestedScopes      []string
}

type OAuthAuthorizationState struct {
	State         string
	CodeChallenge string
	ExpiresAt     time.Time
}

type ConsumedOAuthState struct {
	OrganizationID       uuid.UUID
	AccountID            uuid.UUID
	FlowID               uuid.UUID
	BrowserBindingDigest string
	ConnectionID         *uuid.UUID
	IntegrationID        string
	DriverID             string
	AuthMethodID         string
	RedirectURI          string
	RequestedScopes      []string
	CodeVerifier         string
}

type OAuthStateService struct {
	repository          OAuthStateRepository
	cipher              CredentialCipher
	ttl                 time.Duration
	allowedRedirectURIs map[string]struct{}
}

func (service *OAuthStateService) WithAllowedRedirectURIs(values []string) *OAuthStateService {
	if service == nil {
		return service
	}
	allowed := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if validOAuthRedirectURI(value) {
			allowed[value] = struct{}{}
		}
	}
	service.allowedRedirectURIs = allowed
	return service
}

func NewOAuthStateService(repository OAuthStateRepository, cipher CredentialCipher, ttl time.Duration) *OAuthStateService {
	if ttl <= 0 || ttl > 30*time.Minute {
		ttl = 10 * time.Minute
	}
	return &OAuthStateService{repository: repository, cipher: cipher, ttl: ttl}
}

func (service *OAuthStateService) Create(ctx context.Context, request OAuthStateCreateRequest) (OAuthAuthorizationState, error) {
	if service == nil || service.repository == nil || service.cipher == nil {
		return OAuthAuthorizationState{}, fmt.Errorf("integration OAuth state service is unavailable")
	}
	integrationID := strings.ToLower(strings.TrimSpace(request.IntegrationID))
	driverID := strings.ToLower(strings.TrimSpace(request.DriverID))
	authMethodID := strings.ToLower(strings.TrimSpace(request.AuthMethodID))
	if authMethodID == "" {
		authMethodID = string(AuthMethodTypeOAuth2)
	}
	redirectURI := strings.TrimSpace(request.RedirectURI)
	if request.OrganizationID == uuid.Nil || request.AccountID == uuid.Nil || request.FlowID == uuid.Nil ||
		!validOAuthBrowserBindingDigest(request.BrowserBindingDigest) ||
		!integrationIdentifierPattern.MatchString(integrationID) || !integrationIdentifierPattern.MatchString(driverID) ||
		!integrationIdentifierPattern.MatchString(authMethodID) || len(redirectURI) > 2048 || !validOAuthRedirectURI(redirectURI) {
		return OAuthAuthorizationState{}, invalidInput("integration OAuth state request is invalid", nil)
	}
	if len(service.allowedRedirectURIs) == 0 {
		return OAuthAuthorizationState{}, NewError(ErrorCodeDisabled, "integration OAuth redirect policy is unavailable", nil)
	}
	if _, allowed := service.allowedRedirectURIs[redirectURI]; !allowed {
		return OAuthAuthorizationState{}, invalidInput("integration OAuth redirect URI is not allowed", nil)
	}
	stateValue, err := randomOAuthValue(32)
	if err != nil {
		return OAuthAuthorizationState{}, fmt.Errorf("generate integration OAuth state: %w", err)
	}
	verifier, err := randomOAuthValue(32)
	if err != nil {
		return OAuthAuthorizationState{}, fmt.Errorf("generate integration OAuth PKCE verifier: %w", err)
	}
	stateID := uuid.New()
	expiresAt := time.Now().UTC().Add(service.ttl)
	envelope, err := service.cipher.EncryptCredentials(map[string]string{"pkce_verifier": verifier}, CredentialAAD{
		OrganizationID: request.OrganizationID, ConnectionID: stateID, IntegrationID: integrationID, CredentialVersion: 1,
	})
	if err != nil {
		return OAuthAuthorizationState{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth verifier could not be protected", err)
	}
	state := &IntegrationOAuthState{
		ID: stateID, StateDigest: oauthStateDigest(stateValue), BrowserBindingDigest: request.BrowserBindingDigest,
		OrganizationID: request.OrganizationID,
		AccountID:      request.AccountID, FlowID: request.FlowID, ConnectionID: cloneUUIDPointer(request.ConnectionID),
		IntegrationID: integrationID, DriverID: driverID, AuthMethodID: authMethodID, RedirectURI: redirectURI,
		RequestedScopes: normalizeScopes(request.RequestedScopes), EncryptedVerifier: envelope,
		Status: OAuthStatePending, ExpiresAt: expiresAt,
	}
	if err := service.repository.Create(ctx, state); err != nil {
		return OAuthAuthorizationState{}, err
	}
	challengeDigest := sha256.Sum256([]byte(verifier))
	return OAuthAuthorizationState{
		State: stateValue, CodeChallenge: base64.RawURLEncoding.EncodeToString(challengeDigest[:]), ExpiresAt: expiresAt,
	}, nil
}

func (service *OAuthStateService) Consume(ctx context.Context, rawState, browserBindingDigest string) (ConsumedOAuthState, error) {
	if service == nil || service.repository == nil || service.cipher == nil {
		return ConsumedOAuthState{}, fmt.Errorf("integration OAuth state service is unavailable")
	}
	rawState = strings.TrimSpace(rawState)
	if rawState == "" || len(rawState) > 256 || !validOAuthBrowserBindingDigest(browserBindingDigest) {
		return ConsumedOAuthState{}, NewError(ErrorCodeAuthInvalid, "integration OAuth state is invalid", nil)
	}
	state, err := service.repository.Consume(ctx, oauthStateDigest(rawState), browserBindingDigest, time.Now().UTC())
	if err != nil {
		return ConsumedOAuthState{}, err
	}
	credentials, err := service.cipher.DecryptCredentials(state.EncryptedVerifier, CredentialAAD{
		OrganizationID: state.OrganizationID, ConnectionID: state.ID, IntegrationID: state.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		return ConsumedOAuthState{}, NewError(ErrorCodeAuthInvalid, "integration OAuth verifier is unavailable", err)
	}
	defer destroyCredentialMap(credentials)
	verifier := credentials["pkce_verifier"]
	if verifier == "" {
		return ConsumedOAuthState{}, NewError(ErrorCodeAuthInvalid, "integration OAuth verifier is unavailable", nil)
	}
	return ConsumedOAuthState{
		OrganizationID: state.OrganizationID, AccountID: state.AccountID, FlowID: state.FlowID,
		BrowserBindingDigest: state.BrowserBindingDigest,
		ConnectionID:         cloneUUIDPointer(state.ConnectionID), IntegrationID: state.IntegrationID,
		DriverID: state.DriverID, RedirectURI: state.RedirectURI,
		AuthMethodID:    state.AuthMethodID,
		RequestedScopes: append([]string(nil), state.RequestedScopes...), CodeVerifier: verifier,
	}, nil
}

func validOAuthRedirectURI(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return true
	case "http":
		hostname := strings.ToLower(parsed.Hostname())
		if hostname == "localhost" {
			return true
		}
		address := net.ParseIP(hostname)
		return address != nil && address.IsLoopback()
	default:
		return false
	}
}

func randomOAuthValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	defer zeroBytes(value)
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func oauthStateDigest(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

// OAuthBrowserBindingDigest validates a high-entropy, browser-only binding
// value and returns the only representation that may cross into persistence.
// The raw value must remain confined to the HttpOnly cookie boundary.
func OAuthBrowserBindingDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return "", NewError(ErrorCodeAuthInvalid, "integration OAuth browser binding is invalid", nil)
	}
	digest := sha256.Sum256(decoded)
	return hex.EncodeToString(digest[:]), nil
}

func validOAuthBrowserBindingDigest(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
