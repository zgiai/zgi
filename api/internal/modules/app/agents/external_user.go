package agents

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	// ExternalUserIDHeader is the canonical external Agent end-user identity
	// transport. Integrations should set it at their trusted BFF boundary.
	ExternalUserIDHeader = "X-External-User-ID"

	maxExternalUserIDRunes = 128
	externalUserContextKey = "external_user"
)

var (
	errExternalUserRequired = errors.New("external user id is required")
	errExternalUserMismatch = errors.New("external user id does not match legacy user parameter")
	errExternalUserInvalid  = errors.New("external user id is invalid")
)

// ResolveExternalUserID resolves the canonical header and a temporary legacy
// request value. The returned boolean reports whether the legacy value was used.
func ResolveExternalUserID(headerValue, legacyValue string) (string, bool, error) {
	headerValue = strings.TrimSpace(headerValue)
	legacyValue = strings.TrimSpace(legacyValue)
	if headerValue != "" && legacyValue != "" && headerValue != legacyValue {
		return "", false, errExternalUserMismatch
	}

	value := headerValue
	usedLegacy := false
	if value == "" {
		value = legacyValue
		usedLegacy = value != ""
	}
	if value == "" {
		return "", false, errExternalUserRequired
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxExternalUserIDRunes {
		return "", false, errExternalUserInvalid
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", false, errExternalUserInvalid
		}
	}
	return value, usedLegacy, nil
}

// ExternalUserIDErrorMessage maps identity validation errors to stable public
// messages without exposing diagnostic error text.
func ExternalUserIDErrorMessage(err error) string {
	switch {
	case errors.Is(err, errExternalUserRequired):
		return "external user id is required"
	case errors.Is(err, errExternalUserMismatch):
		return "external user id does not match legacy user parameter"
	default:
		return "external user id is invalid"
	}
}

// DeriveExternalUserID returns a stable, non-reversible identity scoped to one
// workspace and Agent. Raw external user identifiers are never persisted.
func DeriveExternalUserID(workspaceID, agentID uuid.UUID, externalUser string) uuid.UUID {
	seed := strings.Join([]string{
		"zgi",
		// Preserve the original namespace label so existing Agent Memory rows
		// continue to resolve to the same derived user after identity scoping expands.
		"external-agent-memory",
		workspaceID.String(),
		agentID.String(),
		strings.TrimSpace(externalUser),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))
}

func externalUserFromRequest(c *gin.Context, legacyValue string) (string, bool) {
	value, usedLegacy, err := ResolveExternalUserID(c.GetHeader(ExternalUserIDHeader), legacyValue)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, ExternalUserIDErrorMessage(err))
		return "", false
	}
	if usedLegacy {
		c.Header("Deprecation", "true")
		c.Header("Warning", `299 - "the user request parameter is deprecated; use X-External-User-ID"`)
	}
	c.Set(externalUserContextKey, value)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), externalUserContextKey, value))
	return value, true
}
