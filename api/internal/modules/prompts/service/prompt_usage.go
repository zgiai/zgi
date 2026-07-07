package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	"gorm.io/gorm"
)

type promptUsageReferenceQueryRow struct {
	AgentID    string
	AgentName  string
	WorkflowID string
	UpdatedAt  time.Time
	Graph      string
}

type promptUsageRunQueryRow struct {
	WorkflowRunID    *string
	AgentID          string
	AgentName        string
	NodeID           string
	NodeTitle        string
	Status           string
	PromptVersion    *int
	RequestedLabel   *string
	RequestedVersion *int
	CreatedAt        time.Time
	FinishedAt       *time.Time
	ElapsedTime      float64
}

type promptUsageAggregateRow struct {
	TotalRunCount int64
	LastRunAt     *time.Time
}

type promptUsageVersionMetricRow struct {
	Version   int
	RunCount  int64
	LastRunAt *time.Time
}

type promptUsageLabelMetricRow struct {
	Label     string
	RunCount  int64
	LastRunAt *time.Time
}

type promptWorkflowGraph struct {
	Nodes []promptWorkflowGraphNode `json:"nodes"`
}

type promptWorkflowGraphNode struct {
	ID   string                 `json:"id"`
	Data map[string]interface{} `json:"data"`
}

func (s *promptService) GetUsageSummary(ctx context.Context, organizationID, accountID, id string) (*promptdto.PromptUsageSummaryResponse, error) {
	prompt, err := s.getAccessiblePrompt(
		ctx,
		organizationID,
		accountID,
		id,
		promptUsageViewPermissionCodes()...,
	)
	if err != nil {
		return nil, err
	}

	scope, err := shared_visibility.ResolveVisibleWorkspaceScope(
		ctx,
		s.organizationService,
		organizationID,
		accountID,
		derefString(prompt.WorkspaceID),
		promptUsageViewPermissionCodes()...,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve prompt usage scope: %w", err)
	}
	if len(scope.WorkspaceIDs) == 0 {
		return &promptdto.PromptUsageSummaryResponse{
			References: []promptdto.PromptUsageReferenceResponse{},
			RecentRuns: []promptdto.PromptUsageRecentRunResponse{},
		}, nil
	}

	references, linkedAgentCount, err := s.loadPromptUsageReferences(ctx, prompt, scope.WorkspaceIDs)
	if err != nil {
		return nil, err
	}
	aggregate, versionMetrics, labelMetrics, recentRuns, err := s.loadPromptUsageRuns(ctx, prompt, scope.WorkspaceIDs)
	if err != nil {
		return nil, err
	}

	return &promptdto.PromptUsageSummaryResponse{
		LinkedAgentsCount: linkedAgentCount,
		LinkedNodesCount:  int64(len(references)),
		TotalRunCount:     aggregate.TotalRunCount,
		LastRunAt:         aggregate.LastRunAt,
		VersionMetrics:    versionMetrics,
		LabelMetrics:      labelMetrics,
		References:        references,
		RecentRuns:        recentRuns,
	}, nil
}

func (s *promptService) loadPromptUsageReferences(
	ctx context.Context,
	prompt *promptmodel.Prompt,
	workspaceIDs []string,
) ([]promptdto.PromptUsageReferenceResponse, int64, error) {
	var rows []promptUsageReferenceQueryRow
	err := s.repo.DB().WithContext(ctx).
		Table("agents AS a").
		Select("a.id::text AS agent_id, a.name AS agent_name, a.workflow_id::text AS workflow_id, a.updated_at, w.graph").
		Joins("JOIN workflows AS w ON w.id = a.workflow_id AND w.deleted_at IS NULL").
		Where("a.deleted_at IS NULL").
		Where("a.tenant_id IN ?", workspaceIDs).
		Where("w.graph LIKE ?", "%"+prompt.ID+"%").
		Order("a.updated_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("load prompt usage references: %w", err)
	}

	references := make([]promptdto.PromptUsageReferenceResponse, 0)
	linkedAgents := map[string]struct{}{}
	for _, row := range rows {
		nodes, err := extractPromptReferenceNodes(row, prompt.ID)
		if err != nil || len(nodes) == 0 {
			continue
		}
		linkedAgents[row.AgentID] = struct{}{}
		references = append(references, nodes...)
	}

	return references, int64(len(linkedAgents)), nil
}

func extractPromptReferenceNodes(
	row promptUsageReferenceQueryRow,
	promptID string,
) ([]promptdto.PromptUsageReferenceResponse, error) {
	if strings.TrimSpace(row.Graph) == "" {
		return nil, nil
	}

	var graph promptWorkflowGraph
	if err := json.Unmarshal([]byte(row.Graph), &graph); err != nil {
		return nil, err
	}

	results := make([]promptdto.PromptUsageReferenceResponse, 0)
	for _, node := range graph.Nodes {
		reference, ok := node.Data["prompt_reference"].(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(stringFromAny(reference["prompt_id"])) != promptID {
			continue
		}

		item := promptdto.PromptUsageReferenceResponse{
			AgentID:    row.AgentID,
			AgentName:  row.AgentName,
			WorkflowID: row.WorkflowID,
			NodeID:     node.ID,
			NodeTitle:  strings.TrimSpace(stringFromAny(node.Data["title"])),
			UpdatedAt:  row.UpdatedAt,
		}
		if label := strings.TrimSpace(stringFromAny(reference["label"])); label != "" {
			item.ReferenceMode = "label"
			item.Label = stringPtr(label)
		}
		if version := intPtrFromAny(reference["version"]); version != nil {
			item.ReferenceMode = "version"
			item.Version = version
		}
		if item.ReferenceMode == "" {
			item.ReferenceMode = "managed"
		}
		results = append(results, item)
	}

	return results, nil
}

func (s *promptService) loadPromptUsageRuns(
	ctx context.Context,
	prompt *promptmodel.Prompt,
	workspaceIDs []string,
) (promptUsageAggregateRow, []promptdto.PromptUsageVersionMetric, []promptdto.PromptUsageLabelMetric, []promptdto.PromptUsageRecentRunResponse, error) {
	promptIDPattern := fmt.Sprintf(`"managed_prompt_id"\s*:\s*"%s"`, prompt.ID)
	versionExpr := `substring(l.process_data from '"managed_prompt_version"\s*:\s*([0-9]+)')`
	labelExpr := `substring(l.process_data from '"managed_prompt_requested_label"\s*:\s*"([^"]*)"')`
	requestedVersionExpr := `substring(l.process_data from '"managed_prompt_requested_version"\s*:\s*([0-9]+)')`

	baseQuery := s.repo.DB().WithContext(ctx).
		Table("workflow_node_runtime_logs AS l").
		Where("l.deleted_at IS NULL").
		Where("l.tenant_id IN ?", workspaceIDs).
		Where("l.process_data IS NOT NULL").
		Where("l.process_data ~ ?", promptIDPattern)

	var aggregate promptUsageAggregateRow
	if err := baseQuery.Session(&gorm.Session{}).
		Select("COUNT(*) AS total_run_count, MAX(l.created_at) AS last_run_at").
		Scan(&aggregate).Error; err != nil {
		return promptUsageAggregateRow{}, nil, nil, nil, fmt.Errorf("load prompt usage aggregate: %w", err)
	}

	var versionRows []promptUsageVersionMetricRow
	if err := baseQuery.Session(&gorm.Session{}).
		Select(fmt.Sprintf(`
			(%s)::int AS version,
			COUNT(*) AS run_count,
			MAX(l.created_at) AS last_run_at
		`, versionExpr)).
		Where(fmt.Sprintf("%s IS NOT NULL", versionExpr)).
		Group(fmt.Sprintf("(%s)::int", versionExpr)).
		Order(fmt.Sprintf("(%s)::int DESC", versionExpr)).
		Limit(10).
		Scan(&versionRows).Error; err != nil {
		return promptUsageAggregateRow{}, nil, nil, nil, fmt.Errorf("load prompt version metrics: %w", err)
	}

	versionMetrics := make([]promptdto.PromptUsageVersionMetric, 0, len(versionRows))
	for _, row := range versionRows {
		versionMetrics = append(versionMetrics, promptdto.PromptUsageVersionMetric{
			Version:   row.Version,
			RunCount:  row.RunCount,
			LastRunAt: row.LastRunAt,
		})
	}

	var labelRows []promptUsageLabelMetricRow
	if err := baseQuery.Session(&gorm.Session{}).
		Select(fmt.Sprintf(`
			%s AS label,
			COUNT(*) AS run_count,
			MAX(l.created_at) AS last_run_at
		`, labelExpr)).
		Where(fmt.Sprintf("%s IS NOT NULL", labelExpr)).
		Group(labelExpr).
		Order(fmt.Sprintf("COUNT(*) DESC, %s ASC", labelExpr)).
		Limit(10).
		Scan(&labelRows).Error; err != nil {
		return promptUsageAggregateRow{}, nil, nil, nil, fmt.Errorf("load prompt label metrics: %w", err)
	}

	labelMetrics := make([]promptdto.PromptUsageLabelMetric, 0, len(labelRows))
	for _, row := range labelRows {
		labelMetrics = append(labelMetrics, promptdto.PromptUsageLabelMetric{
			Label:     row.Label,
			RunCount:  row.RunCount,
			LastRunAt: row.LastRunAt,
		})
	}

	var rows []promptUsageRunQueryRow
	if err := baseQuery.Session(&gorm.Session{}).
		Select(fmt.Sprintf(`
			l.workflow_run_id::text AS workflow_run_id,
			l.agent_id::text AS agent_id,
			COALESCE(a.name, 'Unknown agent') AS agent_name,
			l.node_id,
			l.title AS node_title,
			l.status,
			(%s)::int AS prompt_version,
			%s AS requested_label,
			(%s)::int AS requested_version,
			l.created_at,
			l.finished_at,
			l.elapsed_time
		`, versionExpr, labelExpr, requestedVersionExpr)).
		Joins("LEFT JOIN agents AS a ON a.id = l.agent_id AND a.deleted_at IS NULL").
		Order("l.created_at DESC").
		Limit(10).
		Scan(&rows).Error; err != nil {
		return promptUsageAggregateRow{}, nil, nil, nil, fmt.Errorf("load prompt recent usage runs: %w", err)
	}

	recentRuns := make([]promptdto.PromptUsageRecentRunResponse, 0, 10)
	for _, row := range rows {
		recentRuns = append(recentRuns, promptdto.PromptUsageRecentRunResponse{
			WorkflowRunID:    row.WorkflowRunID,
			AgentID:          row.AgentID,
			AgentName:        row.AgentName,
			NodeID:           row.NodeID,
			NodeTitle:        row.NodeTitle,
			Status:           row.Status,
			PromptVersion:    row.PromptVersion,
			RequestedLabel:   row.RequestedLabel,
			RequestedVersion: row.RequestedVersion,
			CreatedAt:        row.CreatedAt,
			FinishedAt:       row.FinishedAt,
			ElapsedTime:      row.ElapsedTime,
		})
	}

	return aggregate, versionMetrics, labelMetrics, recentRuns, nil
}

func stringFromAny(value interface{}) string {
	text, _ := value.(string)
	return text
}

func intPtrFromAny(value interface{}) *int {
	switch typed := value.(type) {
	case float64:
		result := int(typed)
		return &result
	case int:
		result := typed
		return &result
	default:
		return nil
	}
}
