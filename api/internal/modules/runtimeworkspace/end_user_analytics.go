package runtimeworkspace

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
)

// End-user quota states a list can filter by.
const (
	QuotaStateOver   = "over"
	QuotaStateNear   = "near"
	QuotaStateCustom = "custom"
)

// nearQuotaRatio is the share of a limit from which an end user is near it.
const nearQuotaRatio = 0.8

// End-user list orders.
var endUserListOrders = map[string]string{
	"last_seen": "last_seen_at",
	"usage_24h": "u24",
	"usage_7d":  "u7",
	"usage_30d": "u30",
	"quota":     "COALESCE(ratio, -1)",
}

// planAppliesSQL holds for an end user whose plan has not expired.
const planAppliesSQL = `(e.plan_key <> '' AND (e.plan_expires_at IS NULL OR e.plan_expires_at > @now))`

// endUserUsageSQL ranks a Runtime Workspace's end users by usage against
// the limits that apply to each: their own when set, else their plan's,
// else the workspace's, and the per-app caps of their plan or of the
// workspace. ratio is the largest share of any limit or cap they have
// spent. Plans and caps are the ones endUserUsageArgs binds, in order.
func endUserUsageSQL(plans, appCaps int, filters string) string {
	limit := func(window string) string {
		var plan strings.Builder
		for index := 0; index < plans; index++ {
			fmt.Fprintf(&plan, `
			WHEN %s AND e.plan_key = @plan_%d THEN CAST(@plan_%d_%s AS numeric)`, planAppliesSQL, index, index, strings.TrimRight(window, "hd"))
		}
		return `CASE
			WHEN COALESCE((e.quota_json->>'unlimited')::boolean, false) THEN NULL
			WHEN jsonb_array_length(COALESCE(e.quota_json->'limits', '[]'::jsonb)) > 0
				THEN (SELECT (item->>'points')::numeric * 1000 FROM jsonb_array_elements(e.quota_json->'limits') AS item
					WHERE item->>'window' = '` + window + `' LIMIT 1)` + plan.String() + `
			ELSE CAST(@` + defaultLimitArg(window) + ` AS numeric)
		END`
	}
	return `
		WITH usage AS (
			SELECT b.end_user_subject_id AS subject_id,
				COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at >= @since_24h), 0) AS u24,
				COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at >= @since_7d), 0) AS u7,
				COALESCE(SUM(b.total_points), 0) AS u30
			FROM llm_usage_bills b
			JOIN runtime_workspace_end_users owner ON owner.subject_id = b.end_user_subject_id
			WHERE owner.runtime_workspace_id = @runtime AND b.request_created_at >= @since_30d
			GROUP BY b.end_user_subject_id
		), limited AS (
			SELECT e.subject_id, e.status, e.last_seen_at, e.quota_json,
				COALESCE(u.u24, 0) AS u24, COALESCE(u.u7, 0) AS u7, COALESCE(u.u30, 0) AS u30,
				` + limit("24h") + ` AS l24,
				` + limit("7d") + ` AS l7,
				` + limit("30d") + ` AS l30
			FROM runtime_workspace_end_users e
			LEFT JOIN usage u ON u.subject_id = e.subject_id
			WHERE e.runtime_workspace_id = @runtime` + filters + `
		)` + appCapsSQL(appCaps) + `, ranked AS (
			SELECT limited.*, GREATEST(limited.u24 / NULLIF(limited.l24, 0), limited.u7 / NULLIF(limited.l7, 0),
				limited.u30 / NULLIF(limited.l30, 0)` + appRatio(appCaps) + `) AS ratio
			FROM limited` + appRatioJoin(appCaps) + `
		)`
}

// appCapsSQL ranks end users against per-app caps: each cap is a row of the
// source it comes from (a plan key, or empty for the workspace), and an end
// user is held to the caps of their applying plan when that plan has its
// own, else to the workspace's.
func appCapsSQL(appCaps int) string {
	if appCaps == 0 {
		return ""
	}
	rows := make([]string, 0, appCaps)
	for index := 0; index < appCaps; index++ {
		rows = append(rows, fmt.Sprintf("(CAST(@cap_source_%d AS text), CAST(@cap_app_%d AS uuid), CAST(@cap_window_%d AS text), CAST(@cap_points_%d AS numeric))", index, index, index, index))
	}
	return `, caps (source, app_id, win, cap) AS (VALUES ` + strings.Join(rows, ", ") + `
		), app_usage AS (
			SELECT b.end_user_subject_id AS subject_id, b.app_id,
				COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at >= @since_24h), 0) AS u24,
				COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at >= @since_7d), 0) AS u7,
				COALESCE(SUM(b.total_points), 0) AS u30
			FROM llm_usage_bills b
			JOIN runtime_workspace_end_users owner ON owner.subject_id = b.end_user_subject_id
			WHERE owner.runtime_workspace_id = @runtime AND b.request_created_at >= @since_30d
				AND b.app_id IN (SELECT app_id FROM caps)
			GROUP BY b.end_user_subject_id, b.app_id
		), app_ratio AS (
			SELECT au.subject_id,
				MAX(CASE c.win WHEN '24h' THEN au.u24 WHEN '7d' THEN au.u7 ELSE au.u30 END / NULLIF(c.cap, 0)) AS ratio
			FROM app_usage au
			JOIN runtime_workspace_end_users e ON e.subject_id = au.subject_id
			JOIN caps c ON c.app_id = au.app_id
				AND c.source = CASE WHEN ` + planAppliesSQL + ` AND e.plan_key IN @cap_plans THEN e.plan_key ELSE '' END
			GROUP BY au.subject_id
		)`
}

func appRatio(appCaps int) string {
	if appCaps == 0 {
		return ""
	}
	return ", app_ratio.ratio"
}

func appRatioJoin(appCaps int) string {
	if appCaps == 0 {
		return ""
	}
	return " LEFT JOIN app_ratio ON app_ratio.subject_id = limited.subject_id"
}

// endUserUsageArgs binds the windows, limits, plans, and per-app caps of a
// Runtime Workspace for endUserUsageSQL, and returns how many caps it bound.
func (s *Service) endUserUsageArgs(ctx context.Context, runtimeWorkspace *RuntimeWorkspace) (map[string]any, int, error) {
	now := s.now()
	args := map[string]any{
		"runtime":   runtimeWorkspace.LogicalID(),
		"since_24h": now.Add(-QuotaWindows["24h"]),
		"since_7d":  now.Add(-QuotaWindows["7d"]),
		"since_30d": now.Add(-QuotaWindows["30d"]),
		"now":       now,
	}
	policy := EndUserQuota(runtimeWorkspace.PolicyJSON)
	for _, window := range QuotaWindowOrder {
		args[defaultLimitArg(window)] = nil
		for index := range policy.Plans {
			args[fmt.Sprintf("plan_%d_%s", index, strings.TrimRight(window, "hd"))] = nil
		}
	}
	for _, limit := range policy.Limits {
		args[defaultLimitArg(limit.Window)] = limit.MilliPoints().Int64()
	}
	for index, plan := range policy.Plans {
		args[fmt.Sprintf("plan_%d", index)] = plan.Key
		for _, limit := range plan.Limits {
			args[fmt.Sprintf("plan_%d_%s", index, strings.TrimRight(limit.Window, "hd"))] = limit.MilliPoints().Int64()
		}
	}

	// Per-app caps name apps by key; the bills name them by ID.
	capPlans := []string{""}
	sources := []struct {
		key  string
		apps []AppQuota
	}{{"", policy.Apps}}
	for _, plan := range policy.Plans {
		if plan.Apps != nil {
			capPlans = append(capPlans, plan.Key)
			sources = append(sources, struct {
				key  string
				apps []AppQuota
			}{plan.Key, *plan.Apps})
		}
	}
	caps := 0
	if len(policy.Apps) > 0 || len(capPlans) > 1 {
		var bindings []AgentBinding
		if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ? AND status = ?", runtimeWorkspace.LogicalID(), AgentBindingActive).Find(&bindings).Error; err != nil {
			return nil, 0, fmt.Errorf("load runtime workspace apps for caps: %w", err)
		}
		appByKey := make(map[string]uuid.UUID, len(bindings))
		for _, binding := range bindings {
			appByKey[binding.PublicAppKey] = binding.AgentID
		}
		for _, source := range sources {
			for _, app := range source.apps {
				appID, ok := appByKey[strings.TrimSpace(app.AppKey)]
				if !ok {
					continue
				}
				for _, limit := range app.Limits {
					args[fmt.Sprintf("cap_source_%d", caps)] = source.key
					args[fmt.Sprintf("cap_app_%d", caps)] = appID
					args[fmt.Sprintf("cap_window_%d", caps)] = limit.Window
					args[fmt.Sprintf("cap_points_%d", caps)] = limit.MilliPoints().Int64()
					caps++
				}
			}
		}
	}
	args["cap_plans"] = capPlans
	return args, caps, nil
}

func planCount(runtimeWorkspace *RuntimeWorkspace) int {
	return len(EndUserQuota(runtimeWorkspace.PolicyJSON).Plans)
}

// defaultLimitArg names the query argument of a window's workspace limit.
func defaultLimitArg(window string) string {
	return "default_" + strings.TrimRight(window, "hd")
}

func endUserListFilters(query EndUserListQuery, args map[string]any) (string, string) {
	var filters, quota []string
	// Erased end users are tombstones: listed only when asked for.
	switch status := strings.TrimSpace(query.Status); status {
	case "":
		filters = append(filters, "e.status <> 'erased'")
	case "all":
	default:
		filters, args["status"] = append(filters, "e.status = @status"), status
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		if subjectID, err := uuid.Parse(search); err == nil {
			filters, args["subject"] = append(filters, "e.subject_id = @subject"), subjectID
		} else {
			filters, args["search"] = append(filters, `e.external_user_id ILIKE @search`), escapeLike(search)+"%"
		}
	}
	switch plan := strings.TrimSpace(query.Plan); {
	case plan == "none":
		filters = append(filters, "NOT "+planAppliesSQL)
	case plan != "":
		filters, args["plan_filter"] = append(filters, planAppliesSQL+" AND e.plan_key = @plan_filter"), plan
	}
	switch query.QuotaState {
	case QuotaStateOver:
		quota = append(quota, "ratio >= 1")
	case QuotaStateNear:
		quota = append(quota, fmt.Sprintf("ratio >= %g AND ratio < 1", nearQuotaRatio))
	case QuotaStateCustom:
		quota = append(quota, "(COALESCE((quota_json->>'unlimited')::boolean, false) OR jsonb_array_length(COALESCE(quota_json->'limits', '[]'::jsonb)) > 0)")
	}
	where := ""
	if len(filters) > 0 {
		where = " AND " + strings.Join(filters, " AND ")
	}
	rankedWhere := ""
	if len(quota) > 0 {
		rankedWhere = " WHERE " + strings.Join(quota, " AND ")
	}
	return where, rankedWhere
}

// listEndUsersRanked lists end users filtered and ordered by their usage.
func (s *Service) listEndUsersRanked(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, query EndUserListQuery, page, limit int) ([]EndUser, int64, error) {
	args, caps, err := s.endUserUsageArgs(ctx, runtimeWorkspace)
	if err != nil {
		return nil, 0, err
	}
	where, rankedWhere := endUserListFilters(query, args)
	base := endUserUsageSQL(planCount(runtimeWorkspace), caps, where)
	var total int64
	if err := s.db.WithContext(ctx).Raw(base+` SELECT count(*) FROM ranked`+rankedWhere, args).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count end users: %w", err)
	}
	order, ok := endUserListOrders[query.Sort]
	if !ok {
		order = endUserListOrders["last_seen"]
	}
	direction := "DESC"
	if query.Ascending {
		direction = "ASC"
	}
	args["limit"], args["offset"] = limit, (page-1)*limit
	var ids []string
	if err := s.db.WithContext(ctx).Raw(base+` SELECT subject_id::text FROM ranked`+rankedWhere+
		` ORDER BY `+order+` `+direction+`, subject_id LIMIT @limit OFFSET @offset`, args).Scan(&ids).Error; err != nil {
		return nil, 0, fmt.Errorf("list end users: %w", err)
	}
	if len(ids) == 0 {
		return []EndUser{}, total, nil
	}
	var users []EndUser
	if err := s.db.WithContext(ctx).Where("subject_id IN ?", ids).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("load end users: %w", err)
	}
	position := make(map[string]int, len(ids))
	for index, id := range ids {
		position[id] = index
	}
	sort.Slice(users, func(i, j int) bool {
		return position[users[i].SubjectID.String()] < position[users[j].SubjectID.String()]
	})
	return users, total, nil
}

// WindowTotal is a Runtime Workspace's end-user usage over one window.
// PreviousPoints is the same window just before it, for comparison.
type WindowTotal struct {
	Window         string  `json:"window"`
	Points         float64 `json:"points"`
	PreviousPoints float64 `json:"previous_points"`
	EndUsers       int64   `json:"end_users"`
}

// EndUserCounts counts a Runtime Workspace's end users.
type EndUserCounts struct {
	Total       int64 `json:"total"`
	Active7d    int64 `json:"active_7d"`
	Suspended   int64 `json:"suspended"`
	Erased      int64 `json:"erased"`
	CustomQuota int64 `json:"custom_quota"`
	OverQuota   int64 `json:"over_quota"`
	NearQuota   int64 `json:"near_quota"`
}

// PlanCount counts the end users a plan applies to.
type PlanCount struct {
	Key      string `json:"key"`
	Name     string `json:"name,omitempty"`
	EndUsers int64  `json:"end_users"`
}

// EndUserSummary is the overview of a Runtime Workspace's end users.
type EndUserSummary struct {
	Usage    []WindowTotal `json:"usage"`
	EndUsers EndUserCounts `json:"end_users"`
	Plans    []PlanCount   `json:"plans"`
	Top      []EndUserView `json:"top"`
}

// SummarizeEndUsers totals usage and counts end users by status and quota.
func (s *Service) SummarizeEndUsers(ctx context.Context, workspaceID uuid.UUID) (*EndUserSummary, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	args, caps, err := s.endUserUsageArgs(ctx, runtimeWorkspace)
	if err != nil {
		return nil, err
	}
	args["active_since"] = s.now().Add(-QuotaWindows["7d"])
	var row struct {
		Total, Active7d, Suspended, Erased, CustomQuota, OverQuota, NearQuota int64
		U24, U7, U30                                                          int64
		Users24, Users7, Users30                                              int64
	}
	if err := s.db.WithContext(ctx).Raw(endUserUsageSQL(planCount(runtimeWorkspace), caps, "")+fmt.Sprintf(`
		SELECT count(*) AS total,
			count(*) FILTER (WHERE last_seen_at >= @active_since AND status <> 'erased') AS active7d,
			count(*) FILTER (WHERE status = 'suspended') AS suspended,
			count(*) FILTER (WHERE status = 'erased') AS erased,
			count(*) FILTER (WHERE COALESCE((quota_json->>'unlimited')::boolean, false) OR jsonb_array_length(COALESCE(quota_json->'limits', '[]'::jsonb)) > 0) AS custom_quota,
			count(*) FILTER (WHERE ratio >= 1) AS over_quota,
			count(*) FILTER (WHERE ratio >= %g AND ratio < 1) AS near_quota,
			COALESCE(sum(u24), 0) AS u24, COALESCE(sum(u7), 0) AS u7, COALESCE(sum(u30), 0) AS u30,
			count(*) FILTER (WHERE u24 > 0) AS users24, count(*) FILTER (WHERE u7 > 0) AS users7, count(*) FILTER (WHERE u30 > 0) AS users30
		FROM ranked`, nearQuotaRatio), args).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("summarize end users: %w", err)
	}
	now := s.now()
	var previous struct{ P24, P7, P30 int64 }
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at >= @from_24 AND b.request_created_at < @since_24h), 0) AS p24,
			COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at >= @from_7 AND b.request_created_at < @since_7d), 0) AS p7,
			COALESCE(SUM(b.total_points) FILTER (WHERE b.request_created_at < @since_30d), 0) AS p30
		FROM llm_usage_bills b
		JOIN runtime_workspace_end_users e ON e.subject_id = b.end_user_subject_id
		WHERE e.runtime_workspace_id = @runtime AND b.request_created_at >= @from_30`,
		map[string]any{"runtime": runtimeWorkspace.LogicalID(),
			"since_24h": args["since_24h"], "since_7d": args["since_7d"], "since_30d": args["since_30d"],
			"from_24": now.Add(-2 * QuotaWindows["24h"]), "from_7": now.Add(-2 * QuotaWindows["7d"]), "from_30": now.Add(-2 * QuotaWindows["30d"]),
		}).Scan(&previous).Error; err != nil {
		return nil, fmt.Errorf("sum previous end-user usage: %w", err)
	}
	summary := &EndUserSummary{
		Usage: []WindowTotal{
			{Window: "24h", Points: pointsOf(millipoints.FromInt64(row.U24)), PreviousPoints: pointsOf(millipoints.FromInt64(previous.P24)), EndUsers: row.Users24},
			{Window: "7d", Points: pointsOf(millipoints.FromInt64(row.U7)), PreviousPoints: pointsOf(millipoints.FromInt64(previous.P7)), EndUsers: row.Users7},
			{Window: "30d", Points: pointsOf(millipoints.FromInt64(row.U30)), PreviousPoints: pointsOf(millipoints.FromInt64(previous.P30)), EndUsers: row.Users30},
		},
		EndUsers: EndUserCounts{Total: row.Total, Active7d: row.Active7d, Suspended: row.Suspended, Erased: row.Erased,
			CustomQuota: row.CustomQuota, OverQuota: row.OverQuota, NearQuota: row.NearQuota},
	}
	var held []struct {
		PlanKey string
		Count   int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT e.plan_key, count(*) AS count FROM runtime_workspace_end_users e
		WHERE e.runtime_workspace_id = @runtime AND e.status <> 'erased' AND `+planAppliesSQL+`
		GROUP BY e.plan_key`, args).Scan(&held).Error; err != nil {
		return nil, fmt.Errorf("count end users per plan: %w", err)
	}
	counts := map[string]int64{}
	for _, row := range held {
		counts[row.PlanKey] = row.Count
	}
	summary.Plans = []PlanCount{}
	for _, plan := range EndUserQuota(runtimeWorkspace.PolicyJSON).Plans {
		summary.Plans = append(summary.Plans, PlanCount{Key: plan.Key, Name: plan.Name, EndUsers: counts[plan.Key]})
	}
	top, _, err := s.listEndUsersRanked(ctx, runtimeWorkspace, EndUserListQuery{Sort: "usage_30d"}, 1, 10)
	if err != nil {
		return nil, err
	}
	used := top[:0]
	for _, user := range top {
		if user.Status != EndUserErased {
			used = append(used, user)
		}
	}
	if summary.Top, err = s.endUserViews(ctx, runtimeWorkspace, used); err != nil {
		return nil, err
	}
	return summary, nil
}

// UsageDay is one day of end-user usage, in total and per app.
// EndUsers counts distinct end users with usage that day, and Calls the
// metered model calls.
type UsageDay struct {
	Date     string             `json:"date"`
	Points   float64            `json:"points"`
	EndUsers int64              `json:"end_users"`
	Calls    int64              `json:"calls"`
	Apps     map[string]float64 `json:"apps"`
}

// UsageSeries is end-user usage per day.
type UsageSeries struct {
	Days []UsageDay `json:"days"`
	Apps []AppUsage `json:"apps"`
}

// EndUserUsageSeries returns daily usage of a Runtime Workspace's end users,
// or of one of them, over the last days in a time zone.
func (s *Service) EndUserUsageSeries(ctx context.Context, workspaceID uuid.UUID, subjectID *uuid.UUID, days int, timezone string) (*UsageSeries, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	if days < 1 || days > 90 {
		days = 30
	}
	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || strings.TrimSpace(timezone) == "" {
		location = time.UTC
	}
	now := s.now().In(location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -(days - 1))
	query := `
		SELECT to_char(b.request_created_at AT TIME ZONE @zone, 'YYYY-MM-DD') AS day, b.app_id::text AS app_id,
			COALESCE(SUM(b.total_points), 0) AS points
		FROM llm_usage_bills b
		JOIN runtime_workspace_end_users e ON e.subject_id = b.end_user_subject_id
		WHERE e.runtime_workspace_id = @runtime AND b.request_created_at >= @start`
	args := map[string]any{"zone": location.String(), "runtime": runtimeWorkspace.LogicalID(), "start": start}
	if subjectID != nil {
		query += ` AND b.end_user_subject_id = @subject`
		args["subject"] = *subjectID
	}
	var rows []struct {
		Day    string
		AppID  *string
		Points int64
	}
	if err := s.db.WithContext(ctx).Raw(query+` GROUP BY 1, 2`, args).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load end-user usage series: %w", err)
	}
	var activity []struct {
		Day      string
		EndUsers int64
		Calls    int64
	}
	activityQuery := strings.Replace(query, `b.app_id::text AS app_id,
			COALESCE(SUM(b.total_points), 0) AS points`, `COUNT(DISTINCT b.end_user_subject_id) AS end_users, COUNT(*) AS calls`, 1)
	if err := s.db.WithContext(ctx).Raw(activityQuery+` GROUP BY 1`, args).Scan(&activity).Error; err != nil {
		return nil, fmt.Errorf("load end-user activity series: %w", err)
	}
	byDay := map[string]*UsageDay{}
	series := &UsageSeries{Days: make([]UsageDay, 0, days), Apps: []AppUsage{}}
	for day := 0; day < days; day++ {
		date := start.AddDate(0, 0, day).Format("2006-01-02")
		series.Days = append(series.Days, UsageDay{Date: date, Apps: map[string]float64{}})
	}
	for index := range series.Days {
		byDay[series.Days[index].Date] = &series.Days[index]
	}
	for _, row := range activity {
		if day, ok := byDay[row.Day]; ok {
			day.EndUsers, day.Calls = row.EndUsers, row.Calls
		}
	}
	seenApps := map[string]bool{}
	for _, row := range rows {
		day, ok := byDay[row.Day]
		if !ok {
			continue
		}
		points := pointsOf(millipoints.FromInt64(row.Points))
		day.Points += points
		if row.AppID != nil {
			day.Apps[*row.AppID] += points
			seenApps[*row.AppID] = true
		}
	}
	var bindings []AgentBinding
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ?", runtimeWorkspace.LogicalID()).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("load runtime workspace apps: %w", err)
	}
	for _, binding := range bindings {
		if seenApps[binding.AgentID.String()] {
			series.Apps = append(series.Apps, AppUsage{AppID: binding.AgentID.String(), AppType: binding.AppType, AppKey: binding.PublicAppKey})
			delete(seenApps, binding.AgentID.String())
		}
	}
	for appID := range seenApps {
		series.Apps = append(series.Apps, AppUsage{AppID: appID})
	}
	return series, nil
}

// EndUserBatchResult reports a batch change.
type EndUserBatchResult struct {
	Updated int      `json:"updated"`
	Failed  []string `json:"failed"`
}

// maxEndUserBatch bounds one batch change.
const maxEndUserBatch = 200

// UpdateEndUsers applies one change to several end users; each is changed
// and recorded on its own.
func (s *Service) UpdateEndUsers(ctx context.Context, workspaceID uuid.UUID, subjectIDs []uuid.UUID, actor EndUserActor, patch EndUserPatch) (*EndUserBatchResult, error) {
	if len(subjectIDs) == 0 || len(subjectIDs) > maxEndUserBatch {
		return nil, ErrInvalidEndUser
	}
	result := &EndUserBatchResult{Failed: []string{}}
	seen := map[uuid.UUID]bool{}
	for _, subjectID := range subjectIDs {
		if seen[subjectID] {
			continue
		}
		seen[subjectID] = true
		if _, err := s.UpdateEndUser(ctx, workspaceID, subjectID, actor, patch); err != nil {
			if err == ErrInvalidEndUser {
				return nil, err
			}
			result.Failed = append(result.Failed, subjectID.String())
			continue
		}
		result.Updated++
	}
	return result, nil
}
