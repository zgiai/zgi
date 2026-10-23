package runtimeworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// End-user states. A suspended end user may read their data but starts no
// new runs; an erased one keeps a row with identifiers only.
const (
	EndUserActive    = "active"
	EndUserSuspended = "suspended"
	EndUserErased    = "erased"
)

// Who changed an end user.
const (
	EndUserActorAccount    = "account"
	EndUserActorCredential = "credential"
)

// End-user event actions.
const (
	EndUserEventSuspended   = "suspended"
	EndUserEventResumed     = "resumed"
	EndUserEventQuotaSet    = "quota_set"
	EndUserEventQuotaReset  = "quota_reset"
	EndUserEventPlanSet     = "plan_set"
	EndUserEventPlanCleared = "plan_cleared"
)

// endUserTouchInterval bounds how often a request rewrites last_seen_at.
const endUserTouchInterval = time.Minute

var (
	ErrEndUserNotFound = errors.New("runtime workspace end user not found")
	ErrInvalidEndUser  = fmt.Errorf("%w: invalid end-user change", ErrInvalidWorkspace)
)

// EndUser is a Runtime Workspace end user in the registry. ExternalUserID is
// what the integrating server sent; it is cleared on erasure.
type EndUser struct {
	SubjectID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"subject_id"`
	RuntimeWorkspaceID uuid.UUID      `gorm:"type:uuid;not null" json:"runtime_workspace_id"`
	ExternalUserID     string         `gorm:"type:varchar(255);not null;default:''" json:"external_user_id"`
	Status             string         `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	QuotaJSON          datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	PlanKey            string         `gorm:"type:varchar(64);not null;default:''" json:"plan_key"`
	PlanExpiresAt      *time.Time     `json:"plan_expires_at,omitempty"`
	CreditsSince       *time.Time     `json:"-"`
	CreditDebt         int64          `gorm:"not null;default:0" json:"-"`
	FirstSeenAt        time.Time      `json:"first_seen_at"`
	LastSeenAt         time.Time      `json:"last_seen_at"`
	SuspendedAt        *time.Time     `json:"suspended_at,omitempty"`
	ErasedAt           *time.Time     `json:"erased_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

func (EndUser) TableName() string { return "runtime_workspace_end_users" }

// EndUserEvent records a change to an end user with identifiers and enum
// values only.
type EndUserEvent struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RuntimeWorkspaceID uuid.UUID      `gorm:"type:uuid;not null" json:"runtime_workspace_id"`
	SubjectID          uuid.UUID      `gorm:"type:uuid;not null" json:"subject_id"`
	Action             string         `gorm:"type:varchar(32);not null" json:"action"`
	ActorKind          string         `gorm:"type:varchar(16);not null" json:"actor_kind"`
	ActorID            *uuid.UUID     `gorm:"type:uuid" json:"actor_id,omitempty"`
	Details            datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	CreatedAt          time.Time      `json:"created_at"`
}

func (EndUserEvent) TableName() string { return "runtime_workspace_end_user_events" }

// EndUserQuotaOverride replaces the Runtime Workspace's per-end-user limits
// for one end user. Unlimited lifts them; per-app caps still apply.
type EndUserQuotaOverride struct {
	Unlimited bool         `json:"unlimited,omitempty"`
	Limits    []QuotaLimit `json:"limits,omitempty"`
}

func (o EndUserQuotaOverride) set() bool { return o.Unlimited || len(o.Limits) > 0 }

func endUserQuotaOverride(raw datatypes.JSON) EndUserQuotaOverride {
	var override EndUserQuotaOverride
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &override)
	}
	return override
}

// Actor identifies who changes an end user.
type EndUserActor struct {
	Kind string
	ID   uuid.UUID
}

// WindowUsage is an end user's points over one rolling window, with the
// limit and what remains when a limit applies.
type WindowUsage struct {
	Window    string   `json:"window"`
	Points    float64  `json:"points"`
	Limit     *float64 `json:"limit,omitempty"`
	Remaining *float64 `json:"remaining,omitempty"`
}

// AppUsage is an end user's points in one app.
type AppUsage struct {
	AppID   string        `json:"app_id"`
	AppType string        `json:"app_type,omitempty"`
	AppKey  string        `json:"app_key,omitempty"`
	Usage   []WindowUsage `json:"usage"`
}

// EndUserPlanView is the plan an end user holds. Active is false once it
// expired or the policy no longer defines it.
type EndUserPlanView struct {
	Key       string     `json:"key"`
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Active    bool       `json:"active"`
}

// EndUserView is an end user as the console and the external API show it.
type EndUserView struct {
	SubjectID      string               `json:"subject_id"`
	ExternalUserID string               `json:"external_user_id"`
	Status         string               `json:"status"`
	FirstSeenAt    time.Time            `json:"first_seen_at"`
	LastSeenAt     time.Time            `json:"last_seen_at"`
	SuspendedAt    *time.Time           `json:"suspended_at,omitempty"`
	QuotaSource    string               `json:"quota_source"`
	Quota          EndUserQuotaOverride `json:"quota"`
	Plan           *EndUserPlanView     `json:"plan,omitempty"`
	Credits        *CreditBalanceView   `json:"credits,omitempty"`
	Usage          []WindowUsage        `json:"usage"`
	Apps           []AppUsage           `json:"apps"`
}

// EndUserList is a page of end users.
type EndUserList struct {
	Items   []EndUserView `json:"items"`
	Page    int           `json:"page"`
	Limit   int           `json:"limit"`
	Total   int64         `json:"total"`
	HasMore bool          `json:"has_more"`
}

// EndUserListQuery filters and orders the end-user list. Sort is one of
// last_seen (the default), usage_24h, usage_7d, usage_30d, or quota;
// QuotaState is over, near, or custom; Plan is a plan key, or "none".
// Status is active, suspended, erased, or all; empty leaves out erased end
// users.
type EndUserListQuery struct {
	Search     string
	Status     string
	QuotaState string
	Plan       string
	Sort       string
	Ascending  bool
	Page       int
	Limit      int
}

// EndUserPlanAssignment gives an end user a plan until an optional expiry.
type EndUserPlanAssignment struct {
	Key       string     `json:"key"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// EndUserPatch changes an end user. A nil field is left as it is; ResetQuota
// returns the end user to the Runtime Workspace's limits, and ClearPlan
// removes their plan.
type EndUserPatch struct {
	Status     *string
	Quota      *EndUserQuotaOverride
	ResetQuota bool
	Plan       *EndUserPlanAssignment
	ClearPlan  bool
}

// touchEndUser records that an end user made a request. It writes at most
// once a minute per end user, and brings an erased end user back.
func (s *Service) touchEndUser(ctx context.Context, runtimeWorkspaceID, subjectID uuid.UUID, externalUserID string) error {
	now := s.now()
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO runtime_workspace_end_users (subject_id, runtime_workspace_id, external_user_id, status, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?)
		ON CONFLICT (subject_id) DO UPDATE SET
			external_user_id = EXCLUDED.external_user_id,
			last_seen_at = EXCLUDED.last_seen_at,
			status = CASE WHEN runtime_workspace_end_users.status = 'erased' THEN 'active' ELSE runtime_workspace_end_users.status END,
			erased_at = NULL,
			updated_at = EXCLUDED.updated_at
		WHERE runtime_workspace_end_users.last_seen_at < ?
			OR runtime_workspace_end_users.external_user_id <> EXCLUDED.external_user_id
			OR runtime_workspace_end_users.status = 'erased'
	`, subjectID, runtimeWorkspaceID, externalUserID, now, now, now, now, now.Add(-endUserTouchInterval)).Error
}

// EnsureEndUser registers an end user the integrating server manages before
// their first request, and brings an erased one back. A registered end user
// is left as it is.
func (s *Service) EnsureEndUser(ctx context.Context, runtimeWorkspaceID, subjectID uuid.UUID, externalUserID string) error {
	now := s.now()
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO runtime_workspace_end_users (subject_id, runtime_workspace_id, external_user_id, status, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?)
		ON CONFLICT (subject_id) DO UPDATE SET
			external_user_id = EXCLUDED.external_user_id,
			status = 'active',
			erased_at = NULL,
			updated_at = EXCLUDED.updated_at
		WHERE runtime_workspace_end_users.status = 'erased'
	`, subjectID, runtimeWorkspaceID, strings.TrimSpace(externalUserID), now, now, now, now).Error
}

// EndUserSubject is the subject of an external user in a Runtime Workspace.
func EndUserSubject(runtimeWorkspaceID uuid.UUID, externalUserID string) (uuid.UUID, error) {
	subject, err := usersubject.RuntimeWorkspaceEndUserSubject(runtimeWorkspaceID, strings.TrimSpace(externalUserID))
	if err != nil {
		return uuid.Nil, ErrEndUserNotFound
	}
	return subject.ID, nil
}

// ListEndUsers lists a Runtime Workspace's end users, most recent first
// unless the query orders them by usage.
func (s *Service) ListEndUsers(ctx context.Context, workspaceID uuid.UUID, query EndUserListQuery) (*EndUserList, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	page, limit := max(query.Page, 1), query.Limit
	if limit < 1 || limit > 200 {
		limit = 50
	}
	users, total, err := s.listEndUsersRanked(ctx, runtimeWorkspace, query, page, limit)
	if err != nil {
		return nil, err
	}
	items, err := s.endUserViews(ctx, runtimeWorkspace, users)
	if err != nil {
		return nil, err
	}
	return &EndUserList{Items: items, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total}, nil
}

// GetEndUser returns one end user of a Runtime Workspace.
func (s *Service) GetEndUser(ctx context.Context, workspaceID, subjectID uuid.UUID) (*EndUserView, error) {
	runtimeWorkspace, user, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return nil, err
	}
	views, err := s.endUserViews(ctx, runtimeWorkspace, []EndUser{*user})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// UpdateEndUser suspends, resumes, or sets the quota of an end user, and
// records each change.
func (s *Service) UpdateEndUser(ctx context.Context, workspaceID, subjectID uuid.UUID, actor EndUserActor, patch EndUserPatch) (*EndUserView, error) {
	if patch.Status != nil && *patch.Status != EndUserActive && *patch.Status != EndUserSuspended {
		return nil, ErrInvalidEndUser
	}
	if patch.Quota != nil && patch.ResetQuota || patch.Plan != nil && (patch.ClearPlan || !ValidPlanKey(patch.Plan.Key)) {
		return nil, ErrInvalidEndUser
	}
	if patch.Quota != nil {
		if !patch.Quota.set() || patch.Quota.Unlimited && len(patch.Quota.Limits) > 0 || !validQuotaLimits(patch.Quota.Limits) {
			return nil, ErrInvalidEndUser
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runtimeWorkspace, user, err := s.loadEndUser(ctx, tx, workspaceID, subjectID, true)
		if err != nil {
			return err
		}
		if user.Status == EndUserErased {
			return ErrEndUserNotFound
		}
		now := s.now()
		// Usage so far is settled under the quota it was incurred under,
		// before that quota changes.
		if patch.Quota != nil || patch.ResetQuota || patch.Plan != nil || patch.ClearPlan {
			if err := settleAllEndUserCredits(ctx, tx, EndUserQuota(runtimeWorkspace.PolicyJSON), user, now); err != nil {
				return err
			}
		}
		updates := map[string]any{}
		var events []EndUserEvent
		event := func(action string, details map[string]any) {
			raw, _ := json.Marshal(details)
			var actorID *uuid.UUID
			if actor.ID != uuid.Nil {
				id := actor.ID
				actorID = &id
			}
			events = append(events, EndUserEvent{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), SubjectID: user.SubjectID,
				Action: action, ActorKind: actor.Kind, ActorID: actorID, Details: datatypes.JSON(raw), CreatedAt: now})
		}
		if patch.Status != nil && *patch.Status != user.Status {
			updates["status"] = *patch.Status
			if *patch.Status == EndUserSuspended {
				updates["suspended_at"] = now
				event(EndUserEventSuspended, map[string]any{})
			} else {
				updates["suspended_at"] = nil
				event(EndUserEventResumed, map[string]any{})
			}
		}
		switch {
		case patch.Quota != nil:
			raw, _ := json.Marshal(patch.Quota)
			updates["quota_json"] = datatypes.JSON(raw)
			windows := make([]string, 0, len(patch.Quota.Limits))
			for _, limit := range patch.Quota.Limits {
				windows = append(windows, limit.Window)
			}
			event(EndUserEventQuotaSet, map[string]any{"unlimited": patch.Quota.Unlimited, "windows": windows})
		case patch.ResetQuota && endUserQuotaOverride(user.QuotaJSON).set():
			updates["quota_json"] = datatypes.JSON("{}")
			event(EndUserEventQuotaReset, map[string]any{})
		}
		switch {
		case patch.Plan != nil:
			if EndUserQuota(runtimeWorkspace.PolicyJSON).Plan(patch.Plan.Key) == nil ||
				patch.Plan.ExpiresAt != nil && !patch.Plan.ExpiresAt.After(now) {
				return ErrInvalidEndUser
			}
			sameExpiry := patch.Plan.ExpiresAt == nil && user.PlanExpiresAt == nil ||
				patch.Plan.ExpiresAt != nil && user.PlanExpiresAt != nil && patch.Plan.ExpiresAt.Equal(*user.PlanExpiresAt)
			if patch.Plan.Key != user.PlanKey || !sameExpiry {
				updates["plan_key"], updates["plan_expires_at"] = patch.Plan.Key, patch.Plan.ExpiresAt
				details := map[string]any{"plan": patch.Plan.Key}
				if patch.Plan.ExpiresAt != nil {
					details["expires_at"] = patch.Plan.ExpiresAt.UTC().Format(time.RFC3339)
				}
				event(EndUserEventPlanSet, details)
			}
		case patch.ClearPlan && user.PlanKey != "":
			updates["plan_key"], updates["plan_expires_at"] = "", nil
			event(EndUserEventPlanCleared, map[string]any{"plan": user.PlanKey})
		}
		if len(updates) == 0 {
			return nil
		}
		updates["updated_at"] = now
		if err := tx.Model(&EndUser{}).Where("subject_id = ?", user.SubjectID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update end user: %w", err)
		}
		if err := tx.Create(&events).Error; err != nil {
			return fmt.Errorf("record end-user events: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetEndUser(ctx, workspaceID, subjectID)
}

// EndUserEventPage is a page of an end user's recorded changes, newest
// first.
type EndUserEventPage struct {
	Items   []EndUserEvent `json:"items"`
	Page    int            `json:"page"`
	Limit   int            `json:"limit"`
	HasMore bool           `json:"has_more"`
}

// ListEndUserEvents lists the recorded changes to an end user, a page at a
// time.
func (s *Service) ListEndUserEvents(ctx context.Context, workspaceID, subjectID uuid.UUID, page, limit int) (*EndUserEventPage, error) {
	runtimeWorkspace, user, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return nil, err
	}
	page = max(page, 1)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var events []EndUserEvent
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ? AND subject_id = ?", runtimeWorkspace.LogicalID(), user.SubjectID).
		Order("created_at DESC").Order("id DESC").Limit(limit + 1).Offset((page - 1) * limit).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list end-user events: %w", err)
	}
	result := &EndUserEventPage{Page: page, Limit: limit, HasMore: len(events) > limit}
	if result.HasMore {
		events = events[:limit]
	}
	result.Items = events
	return result, nil
}

func (s *Service) loadEndUser(ctx context.Context, db *gorm.DB, workspaceID, subjectID uuid.UUID, lock bool) (*RuntimeWorkspace, *EndUser, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, s.endUserLoadError(err)
	}
	query := db.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user EndUser
	if err := query.Where("subject_id = ? AND runtime_workspace_id = ?", subjectID, runtimeWorkspace.LogicalID()).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrEndUserNotFound
		}
		return nil, nil, fmt.Errorf("load end user: %w", err)
	}
	return runtimeWorkspace, &user, nil
}

func (s *Service) endUserLoadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrWorkspaceDisabled
	}
	return fmt.Errorf("load runtime workspace: %w", err)
}

// endUserViews adds usage over every quota window, per app and in total, to
// a page of end users with one query.
func (s *Service) endUserViews(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, users []EndUser) ([]EndUserView, error) {
	views := make([]EndUserView, 0, len(users))
	if len(users) == 0 {
		return views, nil
	}
	subjectIDs := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		subjectIDs = append(subjectIDs, user.SubjectID)
	}
	now := s.now()
	type usageRow struct {
		SubjectID uuid.UUID
		AppID     *uuid.UUID
		Day       int64
		Week      int64
		Month     int64
	}
	var rows []usageRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT end_user_subject_id AS subject_id, app_id,
			COALESCE(SUM(total_points) FILTER (WHERE request_created_at >= ?), 0) AS day,
			COALESCE(SUM(total_points) FILTER (WHERE request_created_at >= ?), 0) AS week,
			COALESCE(SUM(total_points), 0) AS month
		FROM llm_usage_bills
		WHERE end_user_subject_id IN ? AND request_created_at >= ?
		GROUP BY end_user_subject_id, app_id
	`, now.Add(-QuotaWindows["24h"]), now.Add(-QuotaWindows["7d"]), subjectIDs, now.Add(-QuotaWindows["30d"])).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("sum end-user usage: %w", err)
	}
	var bindings []AgentBinding
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ?", runtimeWorkspace.LogicalID()).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("load runtime workspace apps: %w", err)
	}
	bindingByApp := make(map[uuid.UUID]AgentBinding, len(bindings))
	for _, binding := range bindings {
		bindingByApp[binding.AgentID] = binding
	}
	type sums map[string]millipoints.MilliPoints
	add := func(target sums, row usageRow) {
		target["24h"] += millipoints.FromInt64(row.Day)
		target["7d"] += millipoints.FromInt64(row.Week)
		target["30d"] += millipoints.FromInt64(row.Month)
	}
	totals := map[uuid.UUID]sums{}
	perApp := map[uuid.UUID]map[uuid.UUID]sums{}
	for _, row := range rows {
		if totals[row.SubjectID] == nil {
			totals[row.SubjectID], perApp[row.SubjectID] = sums{}, map[uuid.UUID]sums{}
		}
		add(totals[row.SubjectID], row)
		if row.AppID != nil {
			if perApp[row.SubjectID][*row.AppID] == nil {
				perApp[row.SubjectID][*row.AppID] = sums{}
			}
			add(perApp[row.SubjectID][*row.AppID], row)
		}
	}
	policy := EndUserQuota(runtimeWorkspace.PolicyJSON)
	var credited []uuid.UUID
	for _, user := range users {
		if user.CreditsSince != nil {
			credited = append(credited, user.SubjectID)
		}
	}
	lotTotals := map[uuid.UUID]int64{}
	if len(credited) > 0 {
		var totals []struct {
			SubjectID uuid.UUID
			Total     int64
		}
		if err := s.db.WithContext(ctx).Raw(`
			SELECT subject_id, COALESCE(SUM(remaining), 0) AS total FROM runtime_workspace_end_user_credit_lots
			WHERE subject_id IN ? AND remaining > 0 AND (expires_at IS NULL OR expires_at > ?)
			GROUP BY subject_id`, credited, now).Scan(&totals).Error; err != nil {
			return nil, fmt.Errorf("sum end-user credits: %w", err)
		}
		for _, row := range totals {
			lotTotals[row.SubjectID] = row.Total
		}
	}
	windowUsage := func(used sums, limits []QuotaLimit) []WindowUsage {
		usage := make([]WindowUsage, 0, len(QuotaWindowOrder))
		for _, window := range QuotaWindowOrder {
			item := WindowUsage{Window: window, Points: pointsOf(used[window])}
			for _, limit := range limits {
				if limit.Window == window {
					limitPoints := pointsOf(limit.MilliPoints())
					remaining := pointsOf(max(limit.MilliPoints()-used[window], 0))
					item.Limit, item.Remaining = &limitPoints, &remaining
				}
			}
			usage = append(usage, item)
		}
		return usage
	}
	for _, user := range users {
		applied := appliedQuota(policy, &user, now)
		appLimits := map[string][]QuotaLimit{}
		for _, app := range applied.Apps {
			appLimits[strings.TrimSpace(app.AppKey)] = app.Limits
		}
		var plan *EndUserPlanView
		if user.PlanKey != "" {
			plan = &EndUserPlanView{Key: user.PlanKey, ExpiresAt: user.PlanExpiresAt, Active: planApplies(policy, &user, now) != nil}
			if defined := policy.Plan(user.PlanKey); defined != nil {
				plan.Name = defined.Name
			}
		}
		apps := make([]AppUsage, 0, len(perApp[user.SubjectID]))
		for appID, used := range perApp[user.SubjectID] {
			binding := bindingByApp[appID]
			apps = append(apps, AppUsage{AppID: appID.String(), AppType: binding.AppType, AppKey: binding.PublicAppKey,
				Usage: windowUsage(used, appLimits[binding.PublicAppKey])})
		}
		sort.Slice(apps, func(i, j int) bool { return apps[i].Usage[2].Points > apps[j].Usage[2].Points })
		used := totals[user.SubjectID]
		if used == nil {
			used = sums{}
		}
		views = append(views, EndUserView{
			SubjectID: user.SubjectID.String(), ExternalUserID: user.ExternalUserID, Status: user.Status,
			FirstSeenAt: user.FirstSeenAt, LastSeenAt: user.LastSeenAt, SuspendedAt: user.SuspendedAt,
			QuotaSource: applied.Source, Quota: endUserQuotaOverride(user.QuotaJSON), Plan: plan, Usage: windowUsage(used, applied.Limits), Apps: apps,
		})
		if user.CreditsSince != nil {
			views[len(views)-1].Credits = &CreditBalanceView{
				Balance: pointsOf(millipoints.FromInt64(lotTotals[user.SubjectID] - user.CreditDebt)),
				Debt:    pointsOf(millipoints.FromInt64(user.CreditDebt)),
			}
		}
	}
	return views, nil
}

func pointsOf(value millipoints.MilliPoints) float64 {
	points, _ := value.DecimalPoints().Float64()
	return points
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ParseEndUserPatch reads {"status": "...", "quota": {...} | null,
// "plan": {"key": "...", "expires_at": "..."} | null}; a null quota resets
// the end user to the Runtime Workspace's limits, and a null plan removes
// their plan.
func ParseEndUserPatch(body []byte) (EndUserPatch, error) {
	var raw struct {
		Status *string         `json:"status"`
		Quota  json.RawMessage `json:"quota"`
		Plan   json.RawMessage `json:"plan"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return EndUserPatch{}, ErrInvalidEndUser
	}
	patch := EndUserPatch{Status: raw.Status}
	switch plan := strings.TrimSpace(string(raw.Plan)); {
	case plan == "":
	case plan == "null":
		patch.ClearPlan = true
	default:
		var assignment EndUserPlanAssignment
		if err := json.Unmarshal(raw.Plan, &assignment); err != nil || !ValidPlanKey(assignment.Key) {
			return EndUserPatch{}, ErrInvalidEndUser
		}
		patch.Plan = &assignment
	}
	switch quota := strings.TrimSpace(string(raw.Quota)); {
	case quota == "":
	case quota == "null":
		patch.ResetQuota = true
	default:
		var override EndUserQuotaOverride
		if err := json.Unmarshal(raw.Quota, &override); err != nil {
			return EndUserPatch{}, ErrInvalidEndUser
		}
		patch.Quota = &override
	}
	return patch, nil
}
