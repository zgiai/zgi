package digitalhuman

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
)

// ListResourceCandidates projects existing workspace resources without exposing credentials.
func (r *GormRepository) ListResourceCandidates(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ResourceCandidateQuery) ([]ResourceCandidate, error) {
	if _, err := r.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, err
	}
	switch query.Type {
	case resourcebindings.BindingTypeSkill:
		return []ResourceCandidate{}, nil
	case resourcebindings.BindingTypeKnowledgeDataset:
		return r.listKnowledgeCandidates(ctx, scope)
	case resourcebindings.BindingTypeDatabase:
		return r.listDatabaseCandidates(ctx, scope)
	case resourcebindings.BindingTypeDatabaseTable:
		return r.listDatabaseTableCandidates(ctx, scope, query.ParentResourceID)
	case resourcebindings.BindingTypeWorkflow:
		return r.listWorkflowCandidates(ctx, scope)
	case resourcebindings.BindingTypeConnector:
		return r.listConnectorCandidates(ctx, scope, digitalHumanID)
	default:
		return nil, ErrInvalidRequest
	}
}

func (r *GormRepository) listKnowledgeCandidates(ctx context.Context, scope Scope) ([]ResourceCandidate, error) {
	var rows []struct {
		ID          string
		Name        string
		Description string
		Provider    string
		UpdatedAt   time.Time
	}
	err := r.db.WithContext(ctx).Table("datasets").
		Select("id, name, COALESCE(description, '') AS description, provider, updated_at").
		Where("organization_id = ? AND workspace_id = ?", scope.OrganizationID, scope.WorkspaceID).
		Order("LOWER(name) ASC, id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list digital human knowledge candidates: %w", err)
	}
	items := make([]ResourceCandidate, 0, len(rows))
	for _, row := range rows {
		revision, err := hashRevision(struct {
			ID        string    `json:"id"`
			Provider  string    `json:"provider"`
			UpdatedAt time.Time `json:"updated_at"`
		}{row.ID, row.Provider, row.UpdatedAt.UTC()})
		if err != nil {
			return nil, err
		}
		items = append(items, ResourceCandidate{
			Type: resourcebindings.BindingTypeKnowledgeDataset, ResourceID: strings.ToLower(row.ID),
			DisplayName: strings.TrimSpace(row.Name), Description: strings.TrimSpace(row.Description),
			ResourceRevision: revision, AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeRead},
			Metadata: map[string]any{"provider": strings.TrimSpace(row.Provider)}, Status: "ready", Available: true,
		})
	}
	return items, nil
}

func (r *GormRepository) listDatabaseCandidates(ctx context.Context, scope Scope) ([]ResourceCandidate, error) {
	var rows []struct {
		ID          string
		Name        string
		Description string
		Status      string
		UpdatedAt   time.Time
		TableCount  int64
	}
	err := r.db.WithContext(ctx).Table("data_sources AS ds").
		Select(`ds.id, ds.name, COALESCE(ds.description, '') AS description, COALESCE(ds.status, '') AS status, ds.updated_at,
			(SELECT COUNT(*) FROM data_source_tables dst WHERE dst.data_source_id = ds.id) AS table_count`).
		Where("ds.organization_id = ? AND ds.workspace_id = ?", scope.OrganizationID, scope.WorkspaceID).
		Order("LOWER(ds.name) ASC, ds.id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list digital human database candidates: %w", err)
	}
	items := make([]ResourceCandidate, 0, len(rows))
	for _, row := range rows {
		revision, err := hashRevision(struct {
			ID         string    `json:"id"`
			UpdatedAt  time.Time `json:"updated_at"`
			TableCount int64     `json:"table_count"`
		}{row.ID, row.UpdatedAt.UTC(), row.TableCount})
		if err != nil {
			return nil, err
		}
		status := strings.ToLower(strings.TrimSpace(row.Status))
		available := status != "invalid" && status != "disabled" && status != "error"
		items = append(items, ResourceCandidate{
			Type: resourcebindings.BindingTypeDatabase, ResourceID: strings.ToLower(row.ID),
			DisplayName: strings.TrimSpace(row.Name), Description: strings.TrimSpace(row.Description),
			ResourceRevision: revision,
			AccessModes:      []resourcebindings.AccessMode{resourcebindings.AccessModeRead, resourcebindings.AccessModeWrite},
			Metadata:         map[string]any{"table_count": row.TableCount}, Status: status, Available: available,
		})
	}
	return items, nil
}

func (r *GormRepository) listDatabaseTableCandidates(ctx context.Context, scope Scope, dataSourceID string) ([]ResourceCandidate, error) {
	var source struct {
		Name string
	}
	sourceQuery := r.db.WithContext(ctx).Table("data_sources").
		Select("name").
		Where("id = ? AND organization_id = ? AND workspace_id = ?", dataSourceID, scope.OrganizationID, scope.WorkspaceID).
		Scan(&source)
	if sourceQuery.Error != nil {
		return nil, fmt.Errorf("verify digital human database candidate: %w", sourceQuery.Error)
	}
	if sourceQuery.RowsAffected != 1 {
		return nil, ErrResourceBindingInvalid
	}
	var rows []struct {
		ID          string
		Name        string
		Description string
		TableName   string
		UpdatedAt   time.Time
	}
	err := r.db.WithContext(ctx).Table("data_source_tables").
		Select("id, name, COALESCE(description, '') AS description, table_name, updated_at").
		Where("data_source_id = ?", dataSourceID).
		Order("LOWER(name) ASC, id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list digital human database table candidates: %w", err)
	}
	items := make([]ResourceCandidate, 0, len(rows))
	for _, row := range rows {
		revision, err := hashRevision(struct {
			ID        string    `json:"id"`
			TableName string    `json:"table_name"`
			UpdatedAt time.Time `json:"updated_at"`
		}{row.ID, row.TableName, row.UpdatedAt.UTC()})
		if err != nil {
			return nil, err
		}
		items = append(items, ResourceCandidate{
			Type: resourcebindings.BindingTypeDatabaseTable, ResourceID: strings.ToLower(row.ID),
			ParentResourceID: strings.ToLower(dataSourceID), DisplayName: strings.TrimSpace(row.Name),
			Description: strings.TrimSpace(row.Description), ResourceRevision: revision,
			AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeRead, resourcebindings.AccessModeWrite},
			Metadata: map[string]any{
				"physical_table_name": strings.TrimSpace(row.TableName),
				"parent_display_name": strings.TrimSpace(source.Name),
			}, Status: "ready", Available: true,
		})
	}
	return items, nil
}

func (r *GormRepository) listWorkflowCandidates(ctx context.Context, scope Scope) ([]ResourceCandidate, error) {
	var rows []struct {
		AgentID     string
		WorkflowID  string
		VersionUUID string
		Version     string
		AgentType   string
		Name        string
		Description string
		UpdatedAt   time.Time
	}
	err := r.db.WithContext(ctx).Table("workflows").
		Select(`workflows.agent_id, workflows.id AS workflow_id, workflows.version_uuid, workflows.version,
			agents.agent_type, agents.name, COALESCE(agents.description, '') AS description, workflows.created_at AS updated_at`).
		Joins("JOIN agents ON agents.id = workflows.agent_id").
		Where("workflows.tenant_id = ? AND workflows.version != ?", scope.WorkspaceID, "draft").
		Where("agents.deleted_at IS NULL AND agents.web_app_status = ?", "active").
		Where("agents.agent_type IN ?", []string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW"}).
		Order("workflows.agent_id ASC, workflows.created_at DESC, workflows.id DESC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list digital human workflow candidates: %w", err)
	}
	items := make([]ResourceCandidate, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		agentID := strings.ToLower(strings.TrimSpace(row.AgentID))
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		versionUUID := strings.ToLower(strings.TrimSpace(row.VersionUUID))
		metadata := map[string]any{
			"workflow_agent_id": agentID,
			"workflow_id":       strings.ToLower(strings.TrimSpace(row.WorkflowID)),
			"version_uuid":      versionUUID,
			"version":           strings.TrimSpace(row.Version),
			"agent_type":        strings.TrimSpace(row.AgentType),
			"updated_at":        row.UpdatedAt.UTC().Format(time.RFC3339),
		}
		revision, err := hashRevision(metadata)
		if err != nil {
			return nil, err
		}
		items = append(items, ResourceCandidate{
			Type: resourcebindings.BindingTypeWorkflow, ResourceID: agentID,
			DisplayName: strings.TrimSpace(row.Name), Description: strings.TrimSpace(row.Description),
			ResourceRevision: revision, AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeExecute},
			Metadata: metadata, Status: "published", Available: versionUUID != "",
		})
	}
	return items, nil
}
