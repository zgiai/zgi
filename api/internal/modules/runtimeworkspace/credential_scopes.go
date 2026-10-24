package runtimeworkspace

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Credential scopes (ADR 0012). A key without scopes may call everything.
const (
	// CredentialScopeRuntime calls apps, files, and plans as end users.
	CredentialScopeRuntime = "runtime"
	// CredentialScopeEndUsersRead reads end-user state, entitlement,
	// credits, and connections.
	CredentialScopeEndUsersRead = "end_users.read"
	// CredentialScopeEndUsersManage changes end users: plans and quotas,
	// credits, exports, erasure, and connections. It implies read.
	CredentialScopeEndUsersManage = "end_users.manage"

	maxCredentialAppKeys = 500
)

// ErrCredentialScopeDenied reports a call the key's scopes do not allow.
var ErrCredentialScopeDenied = errors.New("runtime workspace credential scope denied")

var credentialScopes = []string{CredentialScopeRuntime, CredentialScopeEndUsersRead, CredentialScopeEndUsersManage}

// CredentialInput is what a new access key is created with.
type CredentialInput struct {
	Name      string
	ExpiresAt *time.Time
	Scopes    []string
	AppKeys   []string
}

func normalizeCredentialLimits(scopes, appKeys []string) ([]string, []string, error) {
	normalizedScopes := []string{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if !slices.Contains(credentialScopes, scope) {
			return nil, nil, ErrInvalidCredential
		}
		if !slices.Contains(normalizedScopes, scope) {
			normalizedScopes = append(normalizedScopes, scope)
		}
	}
	normalizedKeys := []string{}
	for _, key := range appKeys {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 128 {
			return nil, nil, ErrInvalidCredential
		}
		if !slices.Contains(normalizedKeys, key) {
			normalizedKeys = append(normalizedKeys, key)
		}
	}
	// Apps limit what the key runs, so a key limited to apps must run them.
	if len(normalizedKeys) > maxCredentialAppKeys ||
		len(normalizedKeys) > 0 && len(normalizedScopes) > 0 && !slices.Contains(normalizedScopes, CredentialScopeRuntime) {
		return nil, nil, ErrInvalidCredential
	}
	slices.Sort(normalizedScopes)
	slices.Sort(normalizedKeys)
	if len(normalizedScopes) == 0 {
		normalizedScopes = nil
	}
	if len(normalizedKeys) == 0 {
		normalizedKeys = nil
	}
	return normalizedScopes, normalizedKeys, nil
}

// credentialLimitParts adds the limits to the idempotency fingerprint; keys
// without limits keep their earlier fingerprint.
func credentialLimitParts(scopes, appKeys []string) []string {
	if len(scopes) == 0 && len(appKeys) == 0 {
		return nil
	}
	return []string{"scopes=" + strings.Join(scopes, ","), "apps=" + strings.Join(appKeys, ",")}
}

func requireBoundAppKeys(tx *gorm.DB, runtimeWorkspaceID any, appKeys []string) error {
	if len(appKeys) == 0 {
		return nil
	}
	var bound int64
	if err := tx.Model(&AgentBinding{}).Where("runtime_workspace_id = ? AND public_app_key IN ?", runtimeWorkspaceID, appKeys).
		Count(&bound).Error; err != nil {
		return fmt.Errorf("check credential apps: %w", err)
	}
	if int(bound) != len(appKeys) {
		return ErrInvalidCredential
	}
	return nil
}

// Allows reports whether the key may use a scope.
func (c *Credential) Allows(scope string) bool {
	if c == nil {
		return false
	}
	if len(c.Scopes) == 0 {
		return true
	}
	if scope == CredentialScopeEndUsersRead && slices.Contains(c.Scopes, CredentialScopeEndUsersManage) {
		return true
	}
	return slices.Contains(c.Scopes, scope)
}

// AllowsApp reports whether the key may call an app.
func (c *Credential) AllowsApp(appKey string) bool {
	return c != nil && (len(c.AppKeys) == 0 || slices.Contains(c.AppKeys, appKey))
}

// AppsAllowed keeps the apps the key may call; a nil key keeps all.
func (c *Credential) AppsAllowed(apps []App) []App {
	if c == nil || len(c.AppKeys) == 0 {
		return apps
	}
	allowed := make([]App, 0, len(apps))
	for _, app := range apps {
		if c.AllowsApp(app.AppKey) {
			allowed = append(allowed, app)
		}
	}
	return allowed
}
