package digitalhuman

import (
	"errors"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	webAppAnonymousSessionKind = "digital_human_webapp_anonymous"
	webAppSessionIssuer        = "zgi"
	webAppSessionTTL           = 30 * 24 * time.Hour
)

var ErrInvalidWebAppSession = errors.New("invalid digital human webapp session")

type WebAppSessionSigner struct {
	secret []byte
	now    func() time.Time
	ttl    time.Duration
}

type WebAppAnonymousSession struct {
	SubjectID uuid.UUID
	WebAppID  uuid.UUID
	ExpiresAt time.Time
}

type webAppSessionClaims struct {
	Kind      string `json:"kind"`
	WebAppID  string `json:"web_app_id"`
	SubjectID string `json:"subject_id"`
	jwtlib.RegisteredClaims
}

func NewWebAppSessionSigner(secret string) (*WebAppSessionSigner, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrInvalidWebAppSession
	}
	return &WebAppSessionSigner{
		secret: []byte(secret), now: func() time.Time { return time.Now().UTC() }, ttl: webAppSessionTTL,
	}, nil
}

func (s *WebAppSessionSigner) Issue(webAppID uuid.UUID) (string, WebAppAnonymousSession, error) {
	if s == nil || len(s.secret) == 0 || webAppID == uuid.Nil {
		return "", WebAppAnonymousSession{}, ErrInvalidWebAppSession
	}
	now := s.now()
	session := WebAppAnonymousSession{SubjectID: uuid.New(), WebAppID: webAppID, ExpiresAt: now.Add(s.ttl)}
	claims := webAppSessionClaims{
		Kind: webAppAnonymousSessionKind, WebAppID: webAppID.String(), SubjectID: session.SubjectID.String(),
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer: webAppSessionIssuer, Subject: session.SubjectID.String(),
			Audience: jwtlib.ClaimStrings{"digital-human-webapp:" + webAppID.String()},
			IssuedAt: jwtlib.NewNumericDate(now), ExpiresAt: jwtlib.NewNumericDate(session.ExpiresAt),
		},
	}
	token, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", WebAppAnonymousSession{}, err
	}
	return token, session, nil
}

func (s *WebAppSessionSigner) Parse(token string, webAppID uuid.UUID) (WebAppAnonymousSession, error) {
	if s == nil || len(s.secret) == 0 || strings.TrimSpace(token) == "" || webAppID == uuid.Nil {
		return WebAppAnonymousSession{}, ErrInvalidWebAppSession
	}
	claims := &webAppSessionClaims{}
	parsed, err := jwtlib.ParseWithClaims(strings.TrimSpace(token), claims, func(token *jwtlib.Token) (any, error) {
		if token.Method != jwtlib.SigningMethodHS256 {
			return nil, ErrInvalidWebAppSession
		}
		return s.secret, nil
	}, jwtlib.WithIssuer(webAppSessionIssuer), jwtlib.WithAudience("digital-human-webapp:"+webAppID.String()), jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}),
		// Validate expiry on the signer's clock, the one Issue stamped it with.
		jwtlib.WithTimeFunc(s.now))
	if err != nil || parsed == nil || !parsed.Valid || claims.Kind != webAppAnonymousSessionKind || claims.WebAppID != webAppID.String() {
		return WebAppAnonymousSession{}, ErrInvalidWebAppSession
	}
	subjectID, err := uuid.Parse(claims.SubjectID)
	if err != nil || subjectID == uuid.Nil || claims.ExpiresAt == nil {
		return WebAppAnonymousSession{}, ErrInvalidWebAppSession
	}
	return WebAppAnonymousSession{SubjectID: subjectID, WebAppID: webAppID, ExpiresAt: claims.ExpiresAt.Time.UTC()}, nil
}
