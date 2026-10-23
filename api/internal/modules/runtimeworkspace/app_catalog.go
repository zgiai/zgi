package runtimeworkspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

// The console manages many bound apps (ADR 0012): it pages and filters
// them, shows each one's recent usage and per-app caps, adds and removes
// apps without sending the whole selection, and searches candidates on the
// server.

// App list states.
const (
	AppStateCallable    = "callable"
	AppStateUnavailable = "unavailable"
)

// candidateChunk is how many candidates are loaded and authorized at once.
const candidateChunk = 200

// AppListQuery filters and pages the bound apps. Type is an app type, and
// State is callable or unavailable.
type AppListQuery struct {
	Search string
	Type   string
	State  string
	Page   int
	Limit  int
}

// AppUsageStats is an app's end-user usage over the last 30 days.
type AppUsageStats struct {
	Points   float64 `json:"points"`
	Calls    int64   `json:"calls"`
	EndUsers int64   `json:"end_users"`
}

// ManagedApp is a bound app with its recent usage and per-app caps.
type ManagedApp struct {
	ManagementApp
	Usage     AppUsageStats `json:"usage_30d"`
	QuotaCaps []QuotaLimit  `json:"quota_caps"`
}

// AppCounts counts the bound apps by type and availability.
type AppCounts struct {
	All           int `json:"all"`
	Agents        int `json:"agents"`
	DigitalHumans int `json:"digital_humans"`
	Unavailable   int `json:"unavailable"`
}

// ManagedAppPage is a page of the bound apps.
type ManagedAppPage struct {
	Revision int64        `json:"revision"`
	Limit    int          `json:"limit"`
	Counts   AppCounts    `json:"counts"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	HasMore  bool         `json:"has_more"`
	Items    []ManagedApp `json:"items"`
}

func matchesAppSearch(search string, values ...string) bool {
	if search == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}
	return false
}

func pageBounds(page, limit, total int) (int, int, int, int) {
	page = max(page, 1)
	if limit < 1 || limit > 200 {
		limit = 20
	}
	start := min((page-1)*limit, total)
	return page, limit, start, min(start+limit, total)
}

// ListBindingsPage lists a page of the bound apps that match a query, with
// counts over all of them.
func (s *Service) ListBindingsPage(ctx context.Context, workspaceID uuid.UUID, query AppListQuery) (*ManagedAppPage, error) {
	set, err := s.ListBindings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load runtime workspace apps scope: %w", err)
	}
	result := &ManagedAppPage{Revision: set.Revision, Limit: set.Limit, Items: []ManagedApp{}}
	search := strings.ToLower(strings.TrimSpace(query.Search))
	var matched []ManagementApp
	for _, app := range set.Items {
		result.Counts.All++
		switch app.AppType {
		case AppTypeAgent:
			result.Counts.Agents++
		case AppTypeDigitalHuman:
			result.Counts.DigitalHumans++
		}
		if !app.Callable {
			result.Counts.Unavailable++
		}
		if query.Type != "" && app.AppType != query.Type ||
			query.State == AppStateCallable && !app.Callable || query.State == AppStateUnavailable && app.Callable ||
			!matchesAppSearch(search, app.Name, app.AppKey, app.WorkspaceName, app.AgentID) {
			continue
		}
		matched = append(matched, app)
	}
	page, limit, start, end := pageBounds(query.Page, query.Limit, len(matched))
	result.Total, result.Page, result.PageSize, result.HasMore = len(matched), page, limit, end < len(matched)
	pageApps := matched[start:end]
	ids := make([]uuid.UUID, 0, len(pageApps))
	for _, app := range pageApps {
		if id, err := uuid.Parse(app.AgentID); err == nil {
			ids = append(ids, id)
		}
	}
	usage, err := s.appUsage(ctx, runtimeWorkspace, ids)
	if err != nil {
		return nil, err
	}
	caps := map[string][]QuotaLimit{}
	for _, app := range EndUserQuota(runtimeWorkspace.PolicyJSON).Apps {
		caps[strings.TrimSpace(app.AppKey)] = app.Limits
	}
	for _, app := range pageApps {
		managed := ManagedApp{ManagementApp: app, Usage: usage[app.AgentID], QuotaCaps: caps[app.AppKey]}
		if managed.QuotaCaps == nil {
			managed.QuotaCaps = []QuotaLimit{}
		}
		result.Items = append(result.Items, managed)
	}
	return result, nil
}

// appUsage sums the end-user usage of apps over the last 30 days.
func (s *Service) appUsage(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, appIDs []uuid.UUID) (map[string]AppUsageStats, error) {
	usage := map[string]AppUsageStats{}
	if len(appIDs) == 0 {
		return usage, nil
	}
	var rows []struct {
		AppID    string
		Points   int64
		Calls    int64
		EndUsers int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT b.app_id::text AS app_id, COALESCE(SUM(b.total_points), 0) AS points, COUNT(*) AS calls,
			COUNT(DISTINCT b.end_user_subject_id) AS end_users
		FROM llm_usage_bills b
		JOIN runtime_workspace_end_users e ON e.subject_id = b.end_user_subject_id
		WHERE e.runtime_workspace_id = ? AND b.app_id IN ? AND b.request_created_at >= ?
		GROUP BY b.app_id`, runtimeWorkspace.LogicalID(), appIDs, s.now().Add(-QuotaWindows["30d"])).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("sum runtime workspace app usage: %w", err)
	}
	for _, row := range rows {
		usage[row.AppID] = AppUsageStats{Points: pointsOf(millipoints.FromInt64(row.Points)), Calls: row.Calls, EndUsers: row.EndUsers}
	}
	return usage, nil
}

// ChangeAppBindings adds and removes apps of the current selection, so the
// console need not hold the whole selection.
func (s *Service) ChangeAppBindings(ctx context.Context, workspaceID, actingAccountID uuid.UUID, expectedRevision int64, add []AppRef, remove []uuid.UUID) (*ManagementAppSet, error) {
	set, err := s.ListBindings(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if set.Revision != expectedRevision {
		return nil, ErrBindingRevisionConflict
	}
	removed := make(map[uuid.UUID]bool, len(remove))
	for _, id := range remove {
		removed[id] = true
	}
	seen := map[uuid.UUID]bool{}
	refs := make([]AppRef, 0, len(set.Items)+len(add))
	for _, app := range set.Items {
		id, err := uuid.Parse(app.AgentID)
		if err != nil || removed[id] {
			continue
		}
		seen[id] = true
		refs = append(refs, AppRef{Type: app.AppType, ID: id})
	}
	for _, ref := range add {
		if !seen[ref.ID] && !removed[ref.ID] {
			seen[ref.ID] = true
			refs = append(refs, ref)
		}
	}
	return s.SyncAppBindings(ctx, workspaceID, actingAccountID, expectedRevision, refs)
}

// CandidateQuery searches and pages the apps that may be bound. A zero Limit
// returns every match.
type CandidateQuery struct {
	Search       string
	Type         string
	ExcludeBound bool
	Page         int
	Limit        int
}

// ListAppCandidates lists the published apps of the scope's workspaces that
// the Runtime Workspace may bind, matched and ordered by name on the server.
// Candidates are authorized a chunk at a time until the page is full.
func (s *Service) ListAppCandidates(ctx context.Context, runtimeWorkspaceID, accountID uuid.UUID, scope string, sourceWorkspaceID *uuid.UUID, query CandidateQuery) (*AgentCandidateSet, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load runtime workspace agent candidates scope: %w", err)
	}
	access, err := s.sourceWorkspaceAccess(ctx, runtimeWorkspace, accountID)
	if err != nil {
		return nil, err
	}
	workspaceIDs, scope, err := candidateWorkspaces(runtimeWorkspace, access, scope, sourceWorkspaceID)
	if err != nil {
		return nil, err
	}
	sources := make([]SourceWorkspace, 0, len(access.Workspaces))
	if access.CrossEnabled {
		for _, workspace := range access.Workspaces {
			if !workspace.IsCurrent {
				sources = append(sources, workspace)
			}
		}
	}
	result := &AgentCandidateSet{Scope: scope, CrossWorkspaceEnabled: access.CrossEnabled, SourceWorkspaces: sources, Items: []AgentCandidate{}}
	if len(workspaceIDs) == 0 {
		return result, nil
	}

	filters, args := "", map[string]any{"workspaces": workspaceIDs}
	if search := strings.TrimSpace(query.Search); search != "" {
		filters, args["search"] = " AND name ILIKE @search", "%"+escapeLike(search)+"%"
	}
	typeFilter := ""
	switch query.Type {
	case AppTypeAgent, AppTypeDigitalHuman:
		typeFilter, args["type"] = " WHERE app_type = @type", query.Type
	case "":
	default:
		return result, nil
	}
	var rows []struct {
		ID      uuid.UUID
		AppType string
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT id, app_type FROM (
			SELECT id, 'agent' AS app_type, name FROM agents
			WHERE tenant_id IN @workspaces AND deleted_at IS NULL`+filters+`
			UNION ALL
			SELECT id, 'digital_human' AS app_type, name FROM digital_humans
			WHERE workspace_id IN @workspaces AND deleted_at IS NULL`+filters+`
		) candidates`+typeFilter+`
		ORDER BY name, id`, args).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list runtime workspace app candidate ids: %w", err)
	}
	bound := map[uuid.UUID]bool{}
	if query.ExcludeBound {
		var ids []uuid.UUID
		if err := s.db.WithContext(ctx).Model(&AgentBinding{}).
			Where("runtime_workspace_id = ? AND status = ?", runtimeWorkspace.LogicalID(), AgentBindingActive).Pluck("agent_id", &ids).Error; err != nil {
			return nil, fmt.Errorf("list bound runtime workspace apps: %w", err)
		}
		for _, id := range ids {
			bound[id] = true
		}
	}
	page := max(query.Page, 1)
	offset, want := 0, 0
	if query.Limit > 0 {
		offset, want = (page-1)*query.Limit, page*query.Limit+1
	}
	var matched []AgentCandidate
	for start := 0; start < len(rows) && (want == 0 || len(matched) < want); start += candidateChunk {
		chunk := rows[start:min(start+candidateChunk, len(rows))]
		bindings := make([]AgentBinding, 0, len(chunk))
		ids := make([]uuid.UUID, 0, len(chunk))
		for _, row := range chunk {
			if bound[row.ID] {
				continue
			}
			bindings = append(bindings, AgentBinding{RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), AgentID: row.ID, AppType: row.AppType})
			ids = append(ids, row.ID)
		}
		if len(ids) == 0 {
			continue
		}
		agentsByID, authorized, err := s.loadBoundAgents(ctx, runtimeWorkspace, bindings)
		if err != nil {
			return nil, err
		}
		tagsByAgent, err := s.catalogTagsByAgentIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, allowed := authorized[id]; !allowed {
				continue
			}
			matched = append(matched, candidateOf(id, agentsByID[id], tagsByAgent[id]))
		}
	}
	if query.Limit > 0 {
		end := min(offset+query.Limit, len(matched))
		result.HasMore = len(matched) > end
		if offset < end {
			result.Items = matched[offset:end]
		}
		result.Page = page
		return result, nil
	}
	if matched != nil {
		result.Items = matched
	}
	return result, nil
}

func candidateOf(id uuid.UUID, agent runtimeAgentRecord, tags []CatalogTag) AgentCandidate {
	presentation := runtimeAgentPresentation(agent)
	return AgentCandidate{
		AgentID: id.String(), AppType: runtimeAppType(agent), WorkspaceID: agent.TenantID.String(), WorkspaceName: agent.WorkspaceName,
		Name: agent.Name, Description: agent.Description, Icon: agent.Icon, IconType: agent.IconType, IconURL: runtimeAgentIconURL(agent),
		Version: agent.Version, VersionUUID: agent.VersionUUID.String(), PublishedAt: agent.PublishedAt,
		FileUploadEnabled: presentation.FileUploadEnabled, SupportsVision: presentation.SupportsVision,
		AgentMemoryEnabled: presentation.AgentMemoryEnabled && relationshipmemory.RuntimeEnabled(), Capabilities: runtimeAppCapabilities(agent, presentation),
		Tags: tags,
	}
}

// AppPage is a page of the apps /connect/v1 serves.
type AppPage struct {
	Items   []App `json:"items"`
	Page    int   `json:"page"`
	Limit   int   `json:"limit"`
	Total   int   `json:"total"`
	HasMore bool  `json:"has_more"`
}

// ListAppsPage pages the callable apps of a Runtime Workspace.
func (s *Service) ListAppsPage(ctx context.Context, runtimeWorkspaceID uuid.UUID, page, limit int) (*AppPage, error) {
	apps, err := s.ListApps(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, err
	}
	page, limit, start, end := pageBounds(page, limit, len(apps))
	return &AppPage{Items: apps[start:end], Page: page, Limit: limit, Total: len(apps), HasMore: end < len(apps)}, nil
}
