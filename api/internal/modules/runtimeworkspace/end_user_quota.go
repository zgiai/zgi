package runtimeworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/gorm"
)

// Runtime Workspace end users have points quotas over rolling windows
// (ADR 0010). The policy sits next to the request rate limit:
//
//	{"end_user_quota": {
//	   "limits": [{"window": "24h", "points": 200}],
//	   "apps":   [{"app_key": "app_x", "limits": [{"window": "30d", "points": 1000}]}]}}
//
// limits apply to each end user across the Runtime Workspace's apps; apps
// add caps for one app. An unset policy means no quota.

// QuotaWindows are the rolling windows a limit may use.
var QuotaWindows = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// QuotaLimit caps an end user's points over one rolling window. Points may
// be fractional down to one milli-point, since a model call often costs less
// than a point.
type QuotaLimit struct {
	Window string          `json:"window"`
	Points decimal.Decimal `json:"points"`
}

// MilliPoints is the limit in the unit usage bills record.
func (l QuotaLimit) MilliPoints() millipoints.MilliPoints {
	return millipoints.FromDecimalPoints(l.Points)
}

// AppQuota caps an end user's points in one app.
type AppQuota struct {
	AppKey string       `json:"app_key"`
	Limits []QuotaLimit `json:"limits"`
}

// EndUserQuotaPolicy is the quota part of a Runtime Workspace policy.
type EndUserQuotaPolicy struct {
	Limits []QuotaLimit `json:"limits,omitempty"`
	Apps   []AppQuota   `json:"apps,omitempty"`
}

// ErrInvalidQuotaPolicy reports a malformed quota.
var ErrInvalidQuotaPolicy = fmt.Errorf("%w: invalid end-user quota", ErrInvalidWorkspace)

// EndUserQuota reads the quota from a Runtime Workspace policy.
func EndUserQuota(policyJSON string) EndUserQuotaPolicy {
	var policy struct {
		EndUserQuota EndUserQuotaPolicy `json:"end_user_quota"`
	}
	if json.Unmarshal([]byte(policyJSON), &policy) != nil {
		return EndUserQuotaPolicy{}
	}
	return policy.EndUserQuota
}

// ValidateEndUserQuota checks a policy's quota before it is saved.
func ValidateEndUserQuota(policyJSON []byte) error {
	var policy struct {
		EndUserQuota *EndUserQuotaPolicy `json:"end_user_quota"`
	}
	if len(policyJSON) == 0 {
		return nil
	}
	if err := json.Unmarshal(policyJSON, &policy); err != nil {
		return ErrInvalidQuotaPolicy
	}
	if policy.EndUserQuota == nil {
		return nil
	}
	valid := func(limits []QuotaLimit) bool {
		seen := map[string]bool{}
		for _, limit := range limits {
			if _, ok := QuotaWindows[limit.Window]; !ok || limit.MilliPoints() < 1 || seen[limit.Window] {
				return false
			}
			seen[limit.Window] = true
		}
		return true
	}
	if !valid(policy.EndUserQuota.Limits) {
		return ErrInvalidQuotaPolicy
	}
	apps := map[string]bool{}
	for _, app := range policy.EndUserQuota.Apps {
		key := strings.TrimSpace(app.AppKey)
		if key == "" || apps[key] || len(app.Limits) == 0 || !valid(app.Limits) {
			return ErrInvalidQuotaPolicy
		}
		apps[key] = true
	}
	return nil
}

// errEndUserQuotaExhausted is what an end user over their quota sees.
func errEndUserQuotaExhausted() error {
	return apperror.New(catalog.CodeQuotaExceeded, apperror.WithOperation("runtime_workspace.end_user_quota"))
}

// QuotaGuard enforces Runtime Workspace end-user quotas for chat runtimes.
type QuotaGuard struct {
	db  *gorm.DB
	now func() time.Time
}

// NewQuotaGuard builds the quota guard.
func NewQuotaGuard(db *gorm.DB) *QuotaGuard {
	return &QuotaGuard{db: db, now: func() time.Time { return time.Now().UTC() }}
}

var _ chatruntime.EndUserQuotaGuard = (*QuotaGuard)(nil)

// CheckEndUserQuota rejects a model call once the end user has spent a
// quota of their Runtime Workspace. End users of other origins pass.
func (g *QuotaGuard) CheckEndUserQuota(ctx context.Context, ref chatruntime.EndUserQuotaRef) error {
	if g == nil || g.db == nil || ref.SubjectID == uuid.Nil {
		return nil
	}
	runtimeWorkspaceID := ref.RuntimeWorkspaceID
	if runtimeWorkspaceID == uuid.Nil {
		var issuers []uuid.UUID
		if err := g.db.WithContext(ctx).Table("user_subjects").
			Where("id = ? AND kind = ? AND issuer_type = ? AND issuer_id IS NOT NULL", ref.SubjectID, "api_end_user", SubjectIssuerType).
			Pluck("issuer_id", &issuers).Error; err != nil {
			return fmt.Errorf("resolve runtime workspace of end user: %w", err)
		}
		if len(issuers) != 1 {
			return nil
		}
		runtimeWorkspaceID = issuers[0]
	}
	var workspace RuntimeWorkspace
	if err := g.db.WithContext(ctx).Where("runtime_id = ? OR workspace_id = ?", runtimeWorkspaceID, runtimeWorkspaceID).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load runtime workspace quota: %w", err)
	}
	quota := EndUserQuota(workspace.PolicyJSON)
	if err := g.checkLimits(ctx, ref.SubjectID, "", quota.Limits); err != nil {
		return err
	}
	if len(quota.Apps) == 0 || strings.TrimSpace(ref.AppID) == "" {
		return nil
	}
	appID, err := uuid.Parse(ref.AppID)
	if err != nil {
		return nil
	}
	var binding AgentBinding
	if err := g.db.WithContext(ctx).Where("runtime_workspace_id = ? AND agent_id = ?", workspace.LogicalID(), appID).First(&binding).Error; err != nil {
		return nil
	}
	for _, app := range quota.Apps {
		if strings.TrimSpace(app.AppKey) == binding.PublicAppKey {
			return g.checkLimits(ctx, ref.SubjectID, appID.String(), app.Limits)
		}
	}
	return nil
}

func (g *QuotaGuard) checkLimits(ctx context.Context, subjectID uuid.UUID, appID string, limits []QuotaLimit) error {
	for _, limit := range limits {
		window, ok := QuotaWindows[limit.Window]
		if !ok || limit.MilliPoints() < 1 {
			continue
		}
		used, err := g.Usage(ctx, subjectID, appID, g.now().Add(-window))
		if err != nil {
			return err
		}
		if used >= limit.MilliPoints() {
			return errEndUserQuotaExhausted()
		}
	}
	return nil
}

// Usage sums an end user's points since a time, in one app when appID is
// set.
func (g *QuotaGuard) Usage(ctx context.Context, subjectID uuid.UUID, appID string, since time.Time) (millipoints.MilliPoints, error) {
	query := g.db.WithContext(ctx).Table("llm_usage_bills").Select("COALESCE(SUM(total_points), 0)").
		Where("end_user_subject_id = ? AND request_created_at >= ?", subjectID, since)
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	var used int64
	if err := query.Scan(&used).Error; err != nil {
		return 0, fmt.Errorf("sum end-user usage: %w", err)
	}
	return millipoints.FromInt64(used), nil
}
