package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"gorm.io/gorm"
)

// Applier executes one command against the real pre tables.
//
// Every statement is raw SQL issued from this module: the workspace/organization models
// and the payment module stay untouched, which is what keeps this plan merge-safe against
// codex/video-image-rollback-main.
type Applier struct {
	db    *gorm.DB
	store *Store
}

// NewApplier creates the command applier.
func NewApplier(db *gorm.DB, store *Store) *Applier {
	return &Applier{db: db, store: store}
}

// Result is the outcome of applying one command.
type Result struct {
	Before string
	After  string
}

// Apply runs the command in a transaction together with its idempotency row, so a crash
// can never apply a command without recording it (or the reverse).
//
// The caller must have already checked WasApplied; Apply re-checks inside the
// transaction to close the race between the pull loop and a retry.
func (a *Applier) Apply(ctx context.Context, command *CommandEnvelope) (*Result, error) {
	if command == nil {
		return nil, errors.New("command is required")
	}
	// Expired commands are refused and acknowledged as failed without executing.
	if command.ExpiresAt != nil && !command.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrCommandExpired
	}

	var result Result
	// replayed marks the idempotency-key hit so its stored ACK can be returned to the
	// caller; a repeated command id and the insert race stay silent as before.
	replayed := false
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&PreAppliedCommand{}).
			Where("command_id = ?", command.ID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check applied command: %w", err)
		}
		if count > 0 {
			return ErrAlreadyApplied
		}

		// Console can redeliver the same command under a fresh command id. The key on the
		// envelope is the real boundary: a successful ledger row means the effect is already
		// in place, so the stored ACK payload is reused instead of executing a second time.
		// Only successful rows count, so a failed apply stays retryable.
		idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
		if idempotencyKey != "" {
			var applied PreAppliedCommand
			lookup := tx.Where("idempotency_key = ? AND status = ?", idempotencyKey, StatusApplied).
				Limit(1).Find(&applied)
			if lookup.Error != nil {
				return fmt.Errorf("check applied idempotency key: %w", lookup.Error)
			}
			if lookup.RowsAffected > 0 {
				result.Before = applied.BeforeState
				result.After = applied.AfterState
				replayed = true
				return ErrAlreadyApplied
			}
		}

		before, after, applyErr := a.applyWithin(ctx, tx, command)
		result.Before = before
		result.After = after

		entry := &PreAppliedCommand{
			CommandID:      parseUUIDOrZero(command.ID),
			Seq:            command.Seq,
			Type:           command.Type,
			IdempotencyKey: idempotencyKeyOrNil(idempotencyKey),
			Status:         StatusApplied,
			BeforeState:    before,
			AfterState:     after,
			AppliedAt:      time.Now().UTC(),
		}
		if applyErr != nil {
			entry.Status = StatusFailed
			entry.Error = applyErr.Error()
			// Only a successful apply owns the key: a failed apply stays retryable, and the
			// unique index must not reject the retry that then succeeds under the same key.
			entry.IdempotencyKey = nil
		}
		if err := tx.Create(entry).Error; err != nil {
			// A concurrent duplicate insert means another worker already recorded it.
			if isDuplicateKey(err) {
				return ErrAlreadyApplied
			}
			return fmt.Errorf("record applied command: %w", err)
		}
		return applyErr
	})
	if err != nil {
		if replayed {
			return &result, err
		}
		return nil, err
	}
	return &result, nil
}

// idempotencyKeyOrNil keeps a command without a key NULL, so keyless commands never
// occupy the unique key and keep working as before.
func idempotencyKeyOrNil(key string) *string {
	if key == "" {
		return nil
	}
	return &key
}

// ErrAlreadyApplied means the idempotency ledger already holds this command id, or its
// idempotency key. An idempotency-key hit also returns the stored result so the caller can
// re-ACK it; a command-id hit returns no result.
var ErrAlreadyApplied = errors.New("command already applied")

// ErrCommandExpired means the command outlived its expiry and must not execute.
var ErrCommandExpired = errors.New("command expired")

// ErrUnsupportedCommand means console sent a type this build does not know.
var ErrUnsupportedCommand = errors.New("unsupported command type")

func (a *Applier) applyWithin(ctx context.Context, tx *gorm.DB, command *CommandEnvelope) (string, string, error) {
	// Console transports the payload as a JSON document carried in a string field, so it
	// is decoded once here before the type-specific handler reads it.
	payload := map[string]json.RawMessage{}
	if raw := strings.TrimSpace(command.Payload); raw != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return "", "", fmt.Errorf("decode command payload: %w", err)
		}
	}

	switch command.Type {
	case CommandAccountStatusSet:
		return a.applyAccountStatus(ctx, tx, payload)
	case CommandAPIKeySet:
		return a.applyAPIKeyStatus(ctx, tx, payload)
	case CommandAPIKeyQuotaSet:
		return a.applyAPIKeyQuota(ctx, tx, payload)
	case CommandQuotaSet:
		return a.applyWorkspaceQuota(ctx, tx, payload)
	case CommandCredentialDisable:
		return a.applyCredentialDisable(ctx, tx, command, payload)
	case CommandOrganizationMemberCreate:
		return a.applyOrganizationMemberCreate(ctx, tx, payload)
	case CommandOrganizationCreate:
		return a.applyOrganizationCreate(ctx, tx, payload)
	case CommandOrganizationDepartmentCreate:
		return a.applyOrganizationDepartmentCreate(ctx, tx, payload)
	default:
		return "", "", fmt.Errorf("%w: %s", ErrUnsupportedCommand, command.Type)
	}
}

// account.status.set writes organizations.status. Values follow the pre enum:
// active | inactive | archived | deleted.
func (a *Applier) applyAccountStatus(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	status := rawString(payload, "status")
	if status == "" {
		return "", "", errors.New("account.status.set requires a status")
	}
	switch status {
	case "active", "inactive", "archived", "deleted":
	default:
		return "", "", fmt.Errorf("account.status.set received unsupported status %q", status)
	}

	organizationID, _, err := a.commandOrganization(ctx, tx, CommandAccountStatusSet, payload)
	if err != nil {
		return "", "", err
	}
	if organizationID == "" {
		return "", "", errors.New("no organization found for account.status.set")
	}

	before := map[string]string{}
	var current struct {
		Status string `gorm:"column:status"`
	}
	if err := tx.WithContext(ctx).Table("organizations").
		Select("status").Where("id = ?", organizationID).Scan(&current).Error; err != nil {
		return "", "", fmt.Errorf("read organization status: %w", err)
	}
	before["status"] = current.Status

	now := time.Now().UTC()
	if err := tx.WithContext(ctx).Exec(
		`UPDATE organizations SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, organizationID,
	).Error; err != nil {
		return "", "", fmt.Errorf("update organization status: %w", err)
	}

	return marshalMap(before), marshalMap(map[string]string{"status": status, "organization_id": organizationID}), nil
}

// apikey.set changes only the status column; key material never crosses the network.
func (a *Applier) applyAPIKeyStatus(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	keyID := rawString(payload, "key_id")
	status := rawString(payload, "status")
	if keyID == "" {
		return "", "", errors.New("apikey.set requires key_id")
	}
	if status == "" {
		return "", "", errors.New("apikey.set requires status")
	}

	// With a named organization the key must belong to it (plan A2).
	organizationID := rawString(payload, "organization_id")
	before := map[string]string{}
	var current struct {
		Status string `gorm:"column:status"`
	}
	if err := scopeAPIKey(tx.WithContext(ctx).Table("llm_organization_api_keys").
		Select("status").Where("id = ?", keyID), organizationID).Scan(&current).Error; err != nil {
		return "", "", fmt.Errorf("read api key status: %w", err)
	}
	before["status"] = current.Status

	now := time.Now().UTC()
	result := scopeAPIKey(tx.WithContext(ctx).Table("llm_organization_api_keys").
		Where("id = ?", keyID), organizationID).
		Updates(map[string]interface{}{"status": status, "updated_at": now})
	if result.Error != nil {
		return "", "", fmt.Errorf("update api key status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return marshalMap(before), "", fmt.Errorf("api key %s not found", keyID)
	}

	return marshalMap(before), marshalMap(map[string]string{"key_id": keyID, "status": status}), nil
}

// apikey.quota.set writes quota_limit / model_limits / allow_ips and recomputes
// remain_quota as quota_limit - used_quota.
func (a *Applier) applyAPIKeyQuota(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	keyID := rawString(payload, "key_id")
	if keyID == "" {
		return "", "", errors.New("apikey.quota.set requires key_id")
	}

	var current struct {
		Status             string  `gorm:"column:status"`
		QuotaLimit         *int64  `gorm:"column:quota_limit"`
		UsedQuota          int64   `gorm:"column:used_quota"`
		RemainQuota        int64   `gorm:"column:remain_quota"`
		ModelLimitsEnabled bool    `gorm:"column:model_limits_enabled"`
		ModelLimits        *string `gorm:"column:model_limits"`
		AllowIPs           string  `gorm:"column:allow_ips"`
	}
	organizationID := rawString(payload, "organization_id")
	if err := scopeAPIKey(tx.WithContext(ctx).Table("llm_organization_api_keys").
		Select("status, quota_limit, used_quota, remain_quota, model_limits_enabled, model_limits, allow_ips").
		Where("id = ?", keyID), organizationID).
		Scan(&current).Error; err != nil {
		return "", "", fmt.Errorf("read api key quota: %w", err)
	}
	before := map[string]interface{}{
		"quota_limit":  current.QuotaLimit,
		"remain_quota": current.RemainQuota,
		"allow_ips":    current.AllowIPs,
	}

	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	after := map[string]interface{}{"key_id": keyID}

	if raw, present := payload["quota_limit"]; present {
		limit, err := rawInt64(raw)
		if err != nil {
			return "", "", fmt.Errorf("apikey.quota.set quota_limit: %w", err)
		}
		updates["quota_limit"] = limit
		remain := limit - current.UsedQuota
		if remain < 0 {
			remain = 0
		}
		updates["remain_quota"] = remain
		after["quota_limit"] = limit
		after["remain_quota"] = remain
	}
	if raw, present := payload["model_limits"]; present {
		value := strings.TrimSpace(string(raw))
		if value == "" || value == "null" {
			updates["model_limits"] = nil
			updates["model_limits_enabled"] = false
		} else {
			updates["model_limits"] = value
			updates["model_limits_enabled"] = true
		}
		after["model_limits"] = value
	}
	if _, present := payload["allow_ips"]; present {
		value := rawString(payload, "allow_ips")
		updates["allow_ips"] = value
		after["allow_ips"] = value
	}

	result := scopeAPIKey(tx.WithContext(ctx).Table("llm_organization_api_keys").Where("id = ?", keyID), organizationID).Updates(updates)
	if result.Error != nil {
		return "", "", fmt.Errorf("update api key quota: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return marshalMap(before), "", fmt.Errorf("api key %s not found", keyID)
	}
	return marshalMap(before), marshalMap(after), nil
}

// quota.set writes llm_workspace_quotas for scope=workspace. This is the real table the
// pre quota pages and the pre-call checks already read, so no second source of truth is
// introduced.
func (a *Applier) applyWorkspaceQuota(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	scope := rawString(payload, "scope")
	if scope != "" && scope != "workspace" {
		return "", "", fmt.Errorf("quota.set received unsupported scope %q", scope)
	}
	scopeID := rawString(payload, "scope_id")
	if scopeID == "" {
		scopeID = rawString(payload, "workspace_id")
	}
	if scopeID == "" {
		return "", "", errors.New("quota.set requires scope_id")
	}

	organizationID, explicit, err := a.commandOrganization(ctx, tx, CommandQuotaSet, payload)
	if err != nil {
		return "", "", err
	}
	if explicit {
		if err := requireWorkspaceInOrganization(ctx, tx, scopeID, organizationID); err != nil {
			return "", "", err
		}
	}

	var current struct {
		QuotaLimit  *int64 `gorm:"column:quota_limit"`
		RemainQuota int64  `gorm:"column:remain_quota"`
		UsedQuota   int64  `gorm:"column:used_quota"`
	}
	if err := tx.WithContext(ctx).Table("llm_workspace_quotas").
		Select("quota_limit, remain_quota, used_quota").
		Where("workspace_id = ?", scopeID).
		Scan(&current).Error; err != nil {
		return "", "", fmt.Errorf("read workspace quota: %w", err)
	}
	before := map[string]interface{}{
		"quota_limit":  current.QuotaLimit,
		"remain_quota": current.RemainQuota,
	}

	// An absent or null limit means "unlimited".
	var limit *int64
	if raw, present := payload["limit"]; present && strings.TrimSpace(string(raw)) != "null" {
		parsed, err := rawInt64(raw)
		if err != nil {
			return "", "", fmt.Errorf("quota.set limit: %w", err)
		}
		limit = &parsed
	}

	remain := int64(0)
	if limit != nil {
		remain = *limit - current.UsedQuota
		if remain < 0 {
			remain = 0
		}
	}

	now := time.Now().UTC()
	update := tx.WithContext(ctx).Exec(
		`UPDATE llm_workspace_quotas SET quota_limit = ?, remain_quota = ?, updated_at = ?
		 WHERE workspace_id = ? AND organization_id = ?`,
		limit, remain, now, scopeID, organizationID,
	)
	if update.Error != nil {
		return "", "", fmt.Errorf("update workspace quota: %w", update.Error)
	}
	if update.RowsAffected == 0 {
		// The workspace has no quota row yet: create it rather than silently doing nothing.
		if err := tx.WithContext(ctx).Exec(
			`INSERT INTO llm_workspace_quotas
			 (workspace_id, organization_id, used_quota, remain_quota, quota_limit, created_at, updated_at)
			 VALUES (?, ?, 0, ?, ?, ?, ?)`,
			scopeID, organizationID, remain, limit, now, now,
		).Error; err != nil {
			return "", "", fmt.Errorf("create workspace quota: %w", err)
		}
	}

	return marshalMap(before), marshalMap(map[string]interface{}{
		"scope":       "workspace",
		"scope_id":    scopeID,
		"quota_limit": limit,
	}), nil
}

// credential.disable records the console's decision locally and drops the loaded
// credential. The real cut-off happens on the console side; pre only stops presenting it.
func (a *Applier) applyCredentialDisable(ctx context.Context, tx *gorm.DB, command *CommandEnvelope, payload map[string]json.RawMessage) (string, string, error) {
	reason := rawString(payload, "reason")
	now := time.Now().UTC()
	if err := a.store.EnsureStateWith(ctx, tx); err != nil {
		return "", "", err
	}
	if err := tx.WithContext(ctx).Model(&PreSyncState{}).
		Where("id = ?", syncStateSingletonID).
		Updates(map[string]interface{}{
			"credential_status": CredentialStatusStale,
			"last_error":        strings.TrimSpace(reason),
			"updated_at":        now,
		}).Error; err != nil {
		return "", "", fmt.Errorf("record credential disable: %w", err)
	}
	consolecred.Global().Clear()
	return marshalMap(map[string]string{"credential_status": CredentialStatusActive}),
		marshalMap(map[string]string{"credential_status": CredentialStatusStale, "reason": reason}), nil
}

func rawString(payload map[string]json.RawMessage, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func rawInt64(raw json.RawMessage) (int64, error) {
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var asFloat float64
	if err := json.Unmarshal(raw, &asFloat); err == nil {
		return int64(asFloat), nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		var parsed int64
		if _, scanErr := fmt.Sscanf(strings.TrimSpace(asString), "%d", &parsed); scanErr == nil {
			return parsed, nil
		}
	}
	return 0, errors.New("value is not an integer")
}

func marshalMap(value interface{}) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
