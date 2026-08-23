package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
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
	case resourcebindings.BindingTypeIntegrationConnection:
		return r.listIntegrationCandidates(ctx, scope)
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

func (r *GormRepository) listIntegrationCandidates(ctx context.Context, scope Scope) ([]ResourceCandidate, error) {
	var rows []struct {
		ID                string
		IntegrationID     string
		Name              string
		Status            string
		HealthStatus      string
		AuthStatus        string
		ScopeStatus       string
		DriverID          string
		AuthMethodID      string
		Revision          int
		CredentialVersion int
		HealthRevision    int
		VerifiedActionIDs datatypes.JSON
	}
	err := r.db.WithContext(ctx).Table("integration_connections").
		Select(`id, integration_id, name, status, health_status, auth_status, scope_status, driver_id, auth_method_id,
			revision, credential_version, health_revision, verified_action_ids`).
		Where("organization_id = ? AND credential_source = ? AND deleted_at IS NULL", scope.OrganizationID, "organization").
		Order("LOWER(name) ASC, id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list digital human integration candidates: %w", err)
	}
	items := make([]ResourceCandidate, 0, len(rows))
	for _, row := range rows {
		actions := []string{}
		if len(row.VerifiedActionIDs) > 0 {
			_ = json.Unmarshal(row.VerifiedActionIDs, &actions)
		}
		metadata := map[string]any{
			"allowed_action_ids": actions,
			"driver_id":          strings.ToLower(strings.TrimSpace(row.DriverID)),
			"auth_method_id":     strings.ToLower(strings.TrimSpace(row.AuthMethodID)),
			"health_status":      strings.ToLower(strings.TrimSpace(row.HealthStatus)),
		}
		revision, err := hashRevision(struct {
			ID                string   `json:"id"`
			Revision          int      `json:"revision"`
			CredentialVersion int      `json:"credential_version"`
			HealthRevision    int      `json:"health_revision"`
			Actions           []string `json:"allowed_action_ids"`
		}{row.ID, row.Revision, row.CredentialVersion, row.HealthRevision, actions})
		if err != nil {
			return nil, err
		}
		status := strings.ToLower(strings.TrimSpace(row.Status))
		health := strings.ToLower(strings.TrimSpace(row.HealthStatus))
		available := status == "active" && strings.EqualFold(row.AuthStatus, "valid") &&
			strings.EqualFold(row.ScopeStatus, "verified") && health != "unhealthy" && len(actions) > 0
		items = append(items, ResourceCandidate{
			Type: resourcebindings.BindingTypeIntegrationConnection, ResourceID: strings.ToLower(row.ID),
			ParentResourceID: strings.ToLower(strings.TrimSpace(row.IntegrationID)), DisplayName: strings.TrimSpace(row.Name),
			Description:      strings.Join([]string{strings.TrimSpace(row.IntegrationID), health}, " · "),
			ResourceRevision: revision, AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeExecute},
			Metadata: metadata, Status: status, Available: available,
		})
	}
	return items, nil
}
