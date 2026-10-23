package runtimeworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
// add caps for one app. An unset policy means no quota. plans (ADR 0011)
// are alternative limits an end user may hold:
//
//	"plans": [{"key": "pro", "name": "Pro", "limits": [...], "apps": [...]}]

// QuotaWindows are the rolling windows a limit may use.
var QuotaWindows = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// QuotaWindowOrder lists the windows from shortest to longest.
var QuotaWindowOrder = []string{"24h", "7d", "30d"}

// QuotaLimit caps an end user's points over one rolling window. Points may
// be fractional down to one milli-point, since a model call often costs less
// than a point.
type QuotaLimit struct {
	Window string          `json:"window"`
	Points decimal.Decimal `json:"points"`
}

// MarshalJSON writes points as a JSON number, as clients send them.
func (l QuotaLimit) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Window string      `json:"window"`
		Points json.Number `json:"points"`
	}{l.Window, json.Number(l.Points.String())})
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

// EndUserPlan is a set of limits an end user may hold instead of the
// workspace defaults. Its limits replace the per-end-user limits; its apps,
// when present, replace the per-app caps.
type EndUserPlan struct {
	Key    string       `json:"key"`
	Name   string       `json:"name,omitempty"`
	Limits []QuotaLimit `json:"limits"`
	Apps   *[]AppQuota  `json:"apps,omitempty"`
}

// EndUserQuotaPolicy is the quota part of a Runtime Workspace policy.
type EndUserQuotaPolicy struct {
	Limits []QuotaLimit  `json:"limits,omitempty"`
	Apps   []AppQuota    `json:"apps,omitempty"`
	Plans  []EndUserPlan `json:"plans,omitempty"`
}

// Plan returns the plan of a key, or nil.
func (p EndUserQuotaPolicy) Plan(key string) *EndUserPlan {
	for index := range p.Plans {
		if key != "" && p.Plans[index].Key == key {
			return &p.Plans[index]
		}
	}
	return nil
}

var planKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidPlanKey reports whether a plan key is well formed.
func ValidPlanKey(key string) bool { return planKeyPattern.MatchString(key) }

// Quota sources: what the limits that apply to an end user come from.
const (
	QuotaSourceDefault   = "default"
	QuotaSourcePlan      = "plan"
	QuotaSourceCustom    = "custom"
	QuotaSourceUnlimited = "unlimited"
)

// AppliedQuota is what limits an end user: per-end-user limits, per-app
// caps, where they come from, and the plan that applies, if any.
type AppliedQuota struct {
	Limits []QuotaLimit
	Apps   []AppQuota
	Source string
	Plan   *EndUserPlan
}

// planApplies reports whether an end user's plan is defined and unexpired.
func planApplies(policy EndUserQuotaPolicy, user *EndUser, now time.Time) *EndUserPlan {
	if user == nil || user.PlanKey == "" || user.PlanExpiresAt != nil && !user.PlanExpiresAt.After(now) {
		return nil
	}
	return policy.Plan(user.PlanKey)
}

// appliedQuota resolves an end user's limits: their own quota, else an
// applying plan, else the workspace defaults. A nil user has the defaults.
func appliedQuota(policy EndUserQuotaPolicy, user *EndUser, now time.Time) AppliedQuota {
	applied := AppliedQuota{Limits: policy.Limits, Apps: policy.Apps, Source: QuotaSourceDefault}
	if plan := planApplies(policy, user, now); plan != nil {
		applied.Limits, applied.Source, applied.Plan = plan.Limits, QuotaSourcePlan, plan
		if plan.Apps != nil {
			applied.Apps = *plan.Apps
		}
	}
	if user != nil {
		switch override := endUserQuotaOverride(user.QuotaJSON); {
		case override.Unlimited:
			applied.Limits, applied.Source = nil, QuotaSourceUnlimited
		case len(override.Limits) > 0:
			applied.Limits, applied.Source = override.Limits, QuotaSourceCustom
		}
	}
	return applied
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
	if !validQuotaLimits(policy.EndUserQuota.Limits) || !validAppQuotas(policy.EndUserQuota.Apps) {
		return ErrInvalidQuotaPolicy
	}
	plans := map[string]bool{}
	for _, plan := range policy.EndUserQuota.Plans {
		if !ValidPlanKey(plan.Key) || plans[plan.Key] || len([]rune(plan.Name)) > 64 || !validQuotaLimits(plan.Limits) {
			return ErrInvalidQuotaPolicy
		}
		if plan.Apps != nil && !validAppQuotas(*plan.Apps) {
			return ErrInvalidQuotaPolicy
		}
		plans[plan.Key] = true
	}
	return nil
}

func validAppQuotas(quotas []AppQuota) bool {
	apps := map[string]bool{}
	for _, app := range quotas {
		key := strings.TrimSpace(app.AppKey)
		if key == "" || apps[key] || len(app.Limits) == 0 || !validQuotaLimits(app.Limits) {
			return false
		}
		apps[key] = true
	}
	return true
}

// validQuotaLimits reports whether limits use known windows, each once,
// with no negative points and none below a milli-point. A zero limit
// includes nothing: every point is drawn from credits (ADR 0011).
func validQuotaLimits(limits []QuotaLimit) bool {
	seen := map[string]bool{}
	for _, limit := range limits {
		if _, ok := QuotaWindows[limit.Window]; !ok || limit.Points.IsNegative() || limit.Points.IsPositive() && limit.MilliPoints() < 1 || seen[limit.Window] {
			return false
		}
		seen[limit.Window] = true
	}
	return true
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

// CheckEndUserQuota rejects a model call of a suspended end user, or once
// the end user has spent a quota of their Runtime Workspace. End users of
// other origins pass.
func (g *QuotaGuard) CheckEndUserQuota(ctx context.Context, ref chatruntime.EndUserQuotaRef) error {
	if g == nil || g.db == nil || ref.SubjectID == uuid.Nil {
		return nil
	}
	runtimeWorkspaceID := ref.RuntimeWorkspaceID
	var user *EndUser
	var users []EndUser
	if err := g.db.WithContext(ctx).Where("subject_id = ?", ref.SubjectID).Limit(1).Find(&users).Error; err != nil {
		return fmt.Errorf("load end user: %w", err)
	}
	if len(users) == 1 {
		if users[0].Status != EndUserActive {
			return apperror.New(chatruntime.CodeEndUserSuspended, apperror.WithOperation("runtime_workspace.end_user_status"))
		}
		user = &users[0]
		runtimeWorkspaceID = user.RuntimeWorkspaceID
	}
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
	policy := EndUserQuota(workspace.PolicyJSON)
	quota := appliedQuota(policy, user, g.now())
	if err := g.checkLimits(ctx, ref.SubjectID, "", quota.Limits); err != nil {
		// Credits pay for usage beyond the per-end-user limits.
		if user == nil || user.CreditsSince == nil {
			return err
		}
		_, credits, settleErr := settledCredits(ctx, g.db, policy, user.SubjectID, g.now(), false)
		if settleErr != nil {
			return settleErr
		}
		if credits.Balance <= 0 {
			return err
		}
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
		if !ok || limit.MilliPoints() < 0 {
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
