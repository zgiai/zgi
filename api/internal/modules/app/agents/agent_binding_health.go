package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

const (
	agentBindingRevisionConflictCode = "agent_binding_revision_conflict"
	agentBindingsInvalidCode         = "agent_bindings_invalid"
	agentBindingsSuspendedCode       = "agent_bindings_suspended"
	agentRollbackImpactChangedCode   = "agent_rollback_impact_changed"

	agentBindingHealthHealthy = "healthy"
	agentBindingHealthWarning = "warning"
	agentBindingHealthBlocked = "blocked"

	agentBindingStatusActive      = "active"
	agentBindingStatusSuspended   = "suspended"
	agentBindingStatusUnavailable = "unavailable"

	agentBindingReasonOrganizationSkillSuspended = "organization_skill_suspended"
	agentBindingReasonResourceDeletedOrMissing   = "resource_deleted_or_missing"
	agentBindingReasonResourceMovedWorkspace     = "resource_moved_workspace"
	agentBindingReasonAuthorizationRevoked       = "authorization_revoked"
	agentBindingReasonResolutionFailed           = "resolution_failed"
)

type agentBindingAPIError struct {
	Code    string
	Message string
	Data    interface{}
}

var agentBindingSystemSkillCatalog = skills.NewRuntime(nil, nil)

func (e *agentBindingAPIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (s *agentsService) draftBindingState(ctx context.Context, ag *Agent, cfg *AgentsConfig, accountID string) ([]agentbindings.Binding, string, dto.AgentBindingHealth, error) {
	config := agentConfigResponse(ag.ID.String(), cfg)
	rows, err := s.bindingRowsForConfig(ctx, ag, config, agentbindings.ScopeDraft, nil, accountID, time.Now())
	if err != nil {
		return nil, "", dto.AgentBindingHealth{}, err
	}
	if s.agentBindings != nil && s.db != nil {
		ref := agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft}
		existing, listErr := s.agentBindings.ListScope(ctx, ref)
		if listErr != nil {
			return nil, "", dto.AgentBindingHealth{}, listErr
		}
		// Read paths never repair the index. Save, publish, rollback, migrations,
		// and resource lifecycle mutations are the only binding-index writers.
		rows = preserveAgentBindingEvidence(rows, existing)
	}
	health := s.resolveAgentBindingHealth(ctx, ag, accountID, config, rows)
	return rows, agentBindingRevision(rows), health, nil
}

func (s *agentsService) bindingRowsForConfig(
	ctx context.Context,
	ag *Agent,
	config *dto.AgentConfigResponse,
	scope agentbindings.Scope,
	versionUUID *uuid.UUID,
	defaultActor string,
	defaultAuthorizedAt time.Time,
) ([]agentbindings.Binding, error) {
	if ag == nil || config == nil {
		return nil, fmt.Errorf("agent and config are required")
	}
	organizationID := s.organizationIDForAgentWorkspace(ctx, ag.TenantID.String())
	organizationUUID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return nil, fmt.Errorf("invalid agent organization id: %w", err)
	}
	defaultActorUUID, _ := uuid.Parse(strings.TrimSpace(defaultActor))
	defaultActorPtr := uuidPtr(defaultActorUUID)
	defaultAtPtr := timePtr(defaultAuthorizedAt)
	bindings := make([]agentbindings.Binding, 0)
	appendBinding := func(binding agentbindings.Binding, actor string, authorizedAtUnix int64) {
		binding.AgentID = ag.ID
		binding.BindingScope = scope
		binding.OrganizationID = organizationUUID
		binding.WorkspaceID = ag.TenantID
		binding.PublishedVersionUUID = cloneBindingUUID(versionUUID)
		binding.AuthorizedBy = defaultActorPtr
		binding.AuthorizedAt = defaultAtPtr
		if actorUUID, parseErr := uuid.Parse(strings.TrimSpace(actor)); parseErr == nil {
			binding.AuthorizedBy = uuidPtr(actorUUID)
		}
		if authorizedAtUnix > 0 {
			at := time.Unix(authorizedAtUnix, 0)
			binding.AuthorizedAt = &at
		}
		bindings = append(bindings, binding)
	}
	for _, skillID := range normalizeAgentEnabledSkillIDs(config.EnabledSkillIDs) {
		appendBinding(agentbindings.Binding{
			BindingType: agentbindings.BindingTypeSkill,
			ResourceID:  skillID,
			AccessMode:  "execute",
		}, defaultActor, defaultAuthorizedAt.Unix())
	}
	for _, datasetID := range normalizeStringIDs(config.KnowledgeDatasetIDs) {
		appendBinding(agentbindings.Binding{
			BindingType: agentbindings.BindingTypeKnowledgeDataset,
			ResourceID:  datasetID,
			AccessMode:  "read",
		}, config.KnowledgeBoundByAccountID, config.KnowledgeBoundAtUnix)
	}
	for _, database := range normalizeAgentDatabaseBindings(config.DatabaseBindings) {
		appendBinding(agentbindings.Binding{
			BindingType: agentbindings.BindingTypeDatabase,
			ResourceID:  database.DataSourceID,
			AccessMode:  "read",
		}, config.DatabaseBoundByAccountID, config.DatabaseBoundAtUnix)
		writable := stringSet(database.WritableTableIDs)
		for _, tableID := range database.TableIDs {
			accessMode := "read"
			if _, ok := writable[tableID]; ok {
				accessMode = "write"
			}
			appendBinding(agentbindings.Binding{
				BindingType:      agentbindings.BindingTypeDatabaseTable,
				ResourceID:       tableID,
				ParentResourceID: database.DataSourceID,
				AccessMode:       accessMode,
			}, config.DatabaseBoundByAccountID, config.DatabaseBoundAtUnix)
		}
	}
	for _, workflow := range normalizeAgentWorkflowBindings(config.WorkflowBindings) {
		appendBinding(workflowAgentBindingRow(workflow), config.WorkflowBoundByAccountID, config.WorkflowBoundAtUnix)
	}
	sortAgentBindings(bindings)
	return bindings, nil
}

func workflowAgentBindingRow(workflow dto.AgentWorkflowBinding) agentbindings.Binding {
	return agentbindings.Binding{
		BindingType:      agentbindings.BindingTypeWorkflow,
		ResourceID:       workflow.BindingID,
		ParentResourceID: workflow.AgentID,
		DisplayName:      workflow.Label,
		AccessMode:       "execute",
	}
}

func (s *agentsService) resolveAgentBindingHealth(ctx context.Context, ag *Agent, accountID string, config *dto.AgentConfigResponse, rows []agentbindings.Binding) dto.AgentBindingHealth {
	items := make([]dto.AgentBindingHealthItem, 0, len(rows))
	for _, row := range rows {
		item := dto.AgentBindingHealthItem{
			BindingType:      string(row.BindingType),
			ResourceID:       row.ResourceID,
			ParentResourceID: row.ParentResourceID,
			// Index labels are historical audit data, not authorization-bearing
			// presentation data. Hydrate live only after current access succeeds.
			DisplayName: "",
			Status:      agentBindingStatusActive,
			Reason:      "",
			AccessMode:  row.AccessMode,
		}
		item.Status, item.Reason = s.resolveBindingHealthItem(ctx, ag, bindingHealthAccountID(row, accountID), config, row)
		if item.DisplayName == "" && item.Status != agentBindingStatusUnavailable {
			item.DisplayName = s.resolveAgentBindingDisplayName(ctx, row, accountID)
		}
		if item.Status != agentBindingStatusActive {
			if item.Status == agentBindingStatusUnavailable {
				item.Suggestion = "remove_or_replace_binding"
			} else {
				item.Suggestion = "restore_access_or_remove_binding"
			}
		}
		items = append(items, item)
	}
	health := dto.AgentBindingHealth{Status: agentBindingHealthHealthy, Items: items}
	for _, item := range items {
		switch item.Status {
		case agentBindingStatusUnavailable:
			health.UnavailableCount++
		case agentBindingStatusSuspended:
			health.SuspendedCount++
		default:
			health.ActiveCount++
		}
	}
	if health.UnavailableCount > 0 {
		health.Status = agentBindingHealthBlocked
	} else if health.SuspendedCount > 0 {
		health.Status = agentBindingHealthWarning
	}
	return health
}

func (s *agentsService) resolveBindingHealthItem(ctx context.Context, ag *Agent, accountID string, config *dto.AgentConfigResponse, row agentbindings.Binding) (string, string) {
	workspaceID := ag.TenantID.String()
	organizationID := row.OrganizationID.String()
	switch row.BindingType {
	case agentbindings.BindingTypeSkill:
		return s.resolveSkillBindingHealth(ctx, row, accountID)
	case agentbindings.BindingTypeKnowledgeDataset:
		if s.knowledgeRetrievalService == nil {
			return agentBindingStatusActive, ""
		}
		if status, reason, decided := s.resolveStoredResourceLocation(ctx, "datasets", row.ResourceID, organizationID, workspaceID); decided {
			return status, reason
		}
		if err := s.validateKnowledgeBindingGrant(ctx, organizationID, workspaceID, accountID, []string{row.ResourceID}); err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonAuthorizationRevoked
		}
		return agentBindingStatusActive, ""
	case agentbindings.BindingTypeDatabase:
		if s.dataSourceService == nil || s.enterpriseService == nil {
			return agentBindingStatusActive, ""
		}
		if status, reason, decided := s.resolveStoredResourceLocation(ctx, "data_sources", row.ResourceID, organizationID, workspaceID); decided {
			return status, reason
		}
		if err := s.validateDatabaseBindingGrant(ctx, organizationID, workspaceID, accountID, []dto.AgentDatabaseBinding{{DataSourceID: row.ResourceID}}); err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonAuthorizationRevoked
		}
		return agentBindingStatusActive, ""
	case agentbindings.BindingTypeDatabaseTable:
		if s.dataSourceService == nil || s.enterpriseService == nil {
			return agentBindingStatusActive, ""
		}
		if status, reason, decided := s.resolveStoredResourceLocation(ctx, "data_sources", row.ParentResourceID, organizationID, workspaceID); decided {
			return status, reason
		}
		if s.db != nil {
			var count int64
			err := s.db.WithContext(ctx).Table("data_source_tables").
				Where("id = ? AND data_source_id = ? AND organization_id = ?", row.ResourceID, row.ParentResourceID, organizationID).
				Count(&count).Error
			if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && count == 0) {
				return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
			}
			if err != nil {
				return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed
			}
		}
		binding := dto.AgentDatabaseBinding{DataSourceID: row.ParentResourceID, TableIDs: []string{row.ResourceID}}
		if row.AccessMode == "write" {
			binding.WritableTableIDs = []string{row.ResourceID}
		}
		if err := s.validateDatabaseBindingGrant(ctx, organizationID, workspaceID, accountID, []dto.AgentDatabaseBinding{binding}); err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonAuthorizationRevoked
		}
		return agentBindingStatusActive, ""
	case agentbindings.BindingTypeWorkflow:
		if s.db == nil {
			return agentBindingStatusActive, ""
		}
		var configured *dto.AgentWorkflowBinding
		for idx := range config.WorkflowBindings {
			workflow := &config.WorkflowBindings[idx]
			if workflow.BindingID == row.ResourceID && workflow.AgentID == row.ParentResourceID {
				configured = workflow
				break
			}
		}
		if configured == nil {
			return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
		}
		var location struct {
			TenantID     string
			DeletedAt    *time.Time
			WebAppStatus string
		}
		err := s.db.WithContext(ctx).Table("agents").
			Select("tenant_id, deleted_at, web_app_status").
			Where("id = ?", row.ParentResourceID).
			Take(&location).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
		}
		if err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed
		}
		if strings.TrimSpace(location.TenantID) != workspaceID {
			return agentBindingStatusUnavailable, agentBindingReasonResourceMovedWorkspace
		}
		if location.DeletedAt != nil {
			return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
		}
		if NormalizeAgentWebAppStatus(AgentWebAppStatus(location.WebAppStatus)) != AgentWebAppStatusActive {
			return agentBindingStatusUnavailable, agentBindingReasonAuthorizationRevoked
		}
		if err := s.validateWorkflowBindingGrant(ctx, workspaceID, []dto.AgentWorkflowBinding{*configured}); err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonAuthorizationRevoked
		}
		return agentBindingStatusActive, ""
	default:
		return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
	}
}

func (s *agentsService) resolveSkillBindingHealth(ctx context.Context, row agentbindings.Binding, accountID string) (string, string) {
	resourceID := strings.ToLower(strings.TrimSpace(row.ResourceID))
	if s.db != nil {
		isSystemSkill := agentBindingSystemSkillCatalog.SystemSkillExists(resourceID) && skills.IsUserSelectableSystemSkill(resourceID)
		if !isSystemSkill {
			var customSkill runtimemodel.CustomSkill
			err := s.db.WithContext(ctx).Model(&runtimemodel.CustomSkill{}).Select("status").
				Where("organization_id = ? AND skill_id = ? AND deleted_at IS NULL", row.OrganizationID, resourceID).
				Take(&customSkill).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
			}
			if err != nil {
				return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed
			}
			if !strings.EqualFold(strings.TrimSpace(customSkill.Status), runtimemodel.CustomSkillStatusActive) {
				return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
			}
		}
		var total int64
		if err := s.db.WithContext(ctx).Model(&runtimemodel.OrganizationSkillConfig{}).Where("organization_id = ?", row.OrganizationID).Count(&total).Error; err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed
		}
		if total == 0 {
			if !isSystemSkill || !defaultOrganizationSkillEnabled(resourceID) {
				return agentBindingStatusSuspended, agentBindingReasonOrganizationSkillSuspended
			}
			return agentBindingStatusActive, ""
		}
		var policy struct{ Enabled bool }
		err := s.db.WithContext(ctx).Model(&runtimemodel.OrganizationSkillConfig{}).Select("enabled").
			Where("organization_id = ? AND skill_id = ?", row.OrganizationID, resourceID).Take(&policy).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if isSystemSkill && defaultOrganizationSkillEnabled(resourceID) {
				return agentBindingStatusActive, ""
			}
			return agentBindingStatusSuspended, agentBindingReasonOrganizationSkillSuspended
		}
		if err == nil && !policy.Enabled {
			return agentBindingStatusSuspended, agentBindingReasonOrganizationSkillSuspended
		}
		if err != nil {
			return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed
		}
		return agentBindingStatusActive, ""
	}
	if s.chatRuntimeService == nil {
		return agentBindingStatusActive, ""
	}
	accountUUID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return agentBindingStatusUnavailable, agentBindingReasonAuthorizationRevoked
	}
	catalog, err := s.chatRuntimeService.ListSkills(ctx, runtimeservice.Scope{
		OrganizationID: row.OrganizationID,
		AccountID:      accountUUID,
	})
	if err != nil {
		return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed
	}
	for _, item := range catalog {
		if strings.ToLower(strings.TrimSpace(item.ID)) != resourceID {
			continue
		}
		if item.Status == skills.SkillStatusInvalid || !skills.SkillBindableToAgent(item) {
			return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
		}
		if !item.Enabled {
			return agentBindingStatusSuspended, agentBindingReasonOrganizationSkillSuspended
		}
		return agentBindingStatusActive, ""
	}
	return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing
}

func defaultOrganizationSkillEnabled(skillID string) bool {
	switch strings.ToLower(strings.TrimSpace(skillID)) {
	case skills.SkillTime, skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager, skills.SkillFileReader:
		return true
	default:
		return false
	}
}

func (s *agentsService) resolveStoredResourceLocation(ctx context.Context, table, resourceID, organizationID, workspaceID string) (string, string, bool) {
	if s.db == nil {
		return "", "", false
	}
	var location struct {
		OrganizationID string
		WorkspaceID    *string
	}
	err := s.db.WithContext(ctx).Table(table).
		Select("organization_id, workspace_id").
		Where("id = ?", strings.TrimSpace(resourceID)).
		Take(&location).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agentBindingStatusUnavailable, agentBindingReasonResourceDeletedOrMissing, true
	}
	if err != nil {
		return agentBindingStatusUnavailable, agentBindingReasonResolutionFailed, true
	}
	resourceWorkspaceID := strings.TrimSpace(organizationID)
	if location.WorkspaceID != nil && strings.TrimSpace(*location.WorkspaceID) != "" {
		resourceWorkspaceID = strings.TrimSpace(*location.WorkspaceID)
	}
	if strings.TrimSpace(location.OrganizationID) != strings.TrimSpace(organizationID) || resourceWorkspaceID != strings.TrimSpace(workspaceID) {
		return agentBindingStatusUnavailable, agentBindingReasonResourceMovedWorkspace, true
	}
	return "", "", false
}

func bindingHealthAccountID(row agentbindings.Binding, fallback string) string {
	if row.AuthorizedBy != nil && *row.AuthorizedBy != uuid.Nil {
		return row.AuthorizedBy.String()
	}
	return strings.TrimSpace(fallback)
}

func (s *agentsService) resolveAgentBindingDisplayName(ctx context.Context, row agentbindings.Binding, accountID string) string {
	if s.db == nil && row.BindingType != agentbindings.BindingTypeSkill {
		return ""
	}
	organizationID := row.OrganizationID.String()
	workspaceID := row.WorkspaceID.String()
	switch row.BindingType {
	case agentbindings.BindingTypeSkill:
		if s.chatRuntimeService == nil {
			return ""
		}
		accountUUID, err := uuid.Parse(strings.TrimSpace(accountID))
		if err != nil {
			return ""
		}
		catalog, err := s.chatRuntimeService.ListSkills(ctx, runtimeservice.Scope{OrganizationID: row.OrganizationID, AccountID: accountUUID})
		if err != nil {
			return ""
		}
		for _, item := range catalog {
			if strings.EqualFold(strings.TrimSpace(item.ID), strings.TrimSpace(row.ResourceID)) {
				return agentSkillCandidateDisplayName(item, row.ResourceID)
			}
		}
		return ""
	case agentbindings.BindingTypeKnowledgeDataset:
		if s.knowledgeRetrievalService == nil || s.validateKnowledgeBindingGrant(ctx, organizationID, workspaceID, accountID, []string{row.ResourceID}) != nil {
			return ""
		}
		return s.bindingResourceName(ctx, "datasets", row.ResourceID)
	case agentbindings.BindingTypeDatabase:
		if s.dataSourceService == nil || s.enterpriseService == nil || s.validateDatabaseBindingGrant(ctx, organizationID, workspaceID, accountID, []dto.AgentDatabaseBinding{{DataSourceID: row.ResourceID}}) != nil {
			return ""
		}
		return s.bindingResourceName(ctx, "data_sources", row.ResourceID)
	case agentbindings.BindingTypeDatabaseTable:
		if s.dataSourceService == nil || s.enterpriseService == nil || s.validateDatabaseBindingGrant(ctx, organizationID, workspaceID, accountID, []dto.AgentDatabaseBinding{{DataSourceID: row.ParentResourceID, TableIDs: []string{row.ResourceID}}}) != nil {
			return ""
		}
		return s.bindingResourceName(ctx, "data_source_tables", row.ResourceID)
	case agentbindings.BindingTypeWorkflow:
		if s.enterpriseService == nil {
			return ""
		}
		canView, err := s.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionWorkflowView)
		if err != nil || !canView {
			return ""
		}
		return s.bindingResourceName(ctx, "agents", row.ParentResourceID)
	default:
		return ""
	}
}

func (s *agentsService) bindingResourceName(ctx context.Context, table, resourceID string) string {
	if s.db == nil {
		return ""
	}
	var row struct{ Name string }
	if err := s.db.WithContext(ctx).Table(table).Select("name").Where("id = ?", resourceID).Take(&row).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(row.Name)
}

func (s *agentsService) validateIncrementalAgentBindingChanges(
	ctx context.Context,
	ag *Agent,
	accountID string,
	previous *dto.AgentConfigResponse,
	next dto.AgentConfigRequest,
) error {
	if ag == nil || previous == nil {
		return fmt.Errorf("agent and previous config are required")
	}
	workspaceID := ag.TenantID.String()
	organizationID := s.organizationIDForAgentWorkspace(ctx, workspaceID)

	previousSkills := stringSet(normalizeAgentEnabledSkillIDs(previous.EnabledSkillIDs))
	for _, skillID := range normalizeAgentEnabledSkillIDs(next.EnabledSkillIDs) {
		if _, exists := previousSkills[skillID]; exists {
			continue
		}
		if err := s.validateAgentEnabledSkillIDs(ctx, workspaceID, accountID, []string{skillID}); err != nil {
			return err
		}
	}
	previousKnowledge := stringSet(normalizeStringIDs(previous.KnowledgeDatasetIDs))
	for _, datasetID := range normalizeStringIDs(next.KnowledgeDatasetIDs) {
		if _, exists := previousKnowledge[datasetID]; exists {
			continue
		}
		if err := s.validateKnowledgeBindingGrant(ctx, organizationID, workspaceID, accountID, []string{datasetID}); err != nil {
			return err
		}
	}
	previousDatabases := databaseBindingsByID(previous.DatabaseBindings)
	for _, binding := range normalizeAgentDatabaseBindings(next.DatabaseBindings) {
		existing, existed := previousDatabases[binding.DataSourceID]
		validationDelta := databaseBindingValidationDelta(existing, binding, existed)
		if len(validationDelta.TableIDs) == 0 {
			continue
		}
		if err := s.validateDatabaseBindingGrant(ctx, organizationID, workspaceID, accountID, []dto.AgentDatabaseBinding{validationDelta}); err != nil {
			return err
		}
	}
	previousWorkflows := workflowBindingsByID(previous.WorkflowBindings)
	for _, binding := range normalizeAgentWorkflowBindings(next.WorkflowBindings) {
		if existing, ok := previousWorkflows[binding.BindingID]; ok && workflowBindingsEqual([]dto.AgentWorkflowBinding{existing}, []dto.AgentWorkflowBinding{binding}) {
			continue
		}
		if err := s.validateWorkflowBindingGrant(ctx, workspaceID, []dto.AgentWorkflowBinding{binding}); err != nil {
			return err
		}
	}
	return nil
}

// databaseBindingValidationDelta returns only newly granted table access. Existing
// read access, unchanged write access, removals, and write-to-read downgrades do
// not need to be reauthorized and must not be blocked by stale resource health.
func databaseBindingValidationDelta(previous, next dto.AgentDatabaseBinding, existed bool) dto.AgentDatabaseBinding {
	normalizedNext := normalizeAgentDatabaseBindings([]dto.AgentDatabaseBinding{next})
	if len(normalizedNext) == 0 {
		return dto.AgentDatabaseBinding{}
	}
	next = normalizedNext[0]
	if !existed {
		return next
	}
	normalizedPrevious := normalizeAgentDatabaseBindings([]dto.AgentDatabaseBinding{previous})
	if len(normalizedPrevious) == 0 {
		return next
	}
	previous = normalizedPrevious[0]
	previousTables := stringSet(previous.TableIDs)
	previousWritable := stringSet(previous.WritableTableIDs)
	nextWritable := stringSet(next.WritableTableIDs)
	delta := dto.AgentDatabaseBinding{DataSourceID: next.DataSourceID, TableIDs: []string{}, WritableTableIDs: []string{}}
	for _, tableID := range next.TableIDs {
		_, tableExisted := previousTables[tableID]
		_, wasWritable := previousWritable[tableID]
		_, isWritable := nextWritable[tableID]
		if tableExisted && (!isWritable || wasWritable) {
			continue
		}
		delta.TableIDs = append(delta.TableIDs, tableID)
		if isWritable {
			delta.WritableTableIDs = append(delta.WritableTableIDs, tableID)
		}
	}
	return delta
}

func databaseBindingsByID(bindings []dto.AgentDatabaseBinding) map[string]dto.AgentDatabaseBinding {
	result := make(map[string]dto.AgentDatabaseBinding, len(bindings))
	for _, binding := range normalizeAgentDatabaseBindings(bindings) {
		result[binding.DataSourceID] = binding
	}
	return result
}

func workflowBindingsByID(bindings []dto.AgentWorkflowBinding) map[string]dto.AgentWorkflowBinding {
	result := make(map[string]dto.AgentWorkflowBinding, len(bindings))
	for _, binding := range normalizeAgentWorkflowBindings(bindings) {
		result[binding.BindingID] = binding
	}
	return result
}

func filterAgentBindingRowsByHealth(rows []agentbindings.Binding, health dto.AgentBindingHealth) []agentbindings.Binding {
	active := make(map[string]struct{}, health.ActiveCount)
	for _, item := range health.Items {
		if item.Status != agentBindingStatusActive {
			continue
		}
		active[agentBindingItemKey(item.BindingType, item.ParentResourceID, item.ResourceID, item.AccessMode)] = struct{}{}
	}
	filtered := make([]agentbindings.Binding, 0, len(rows))
	for _, row := range rows {
		if _, ok := active[agentBindingItemKey(string(row.BindingType), row.ParentResourceID, row.ResourceID, row.AccessMode)]; ok {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func publishedAgentBindingRows(rows []agentbindings.Binding) []agentbindings.Binding {
	return append([]agentbindings.Binding(nil), rows...)
}

func agentBindingResourceRefs(rows []agentbindings.Binding) []agentbindings.ResourceRef {
	refs := make([]agentbindings.ResourceRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, agentbindings.ResourceRef{
			OrganizationID:   row.OrganizationID,
			BindingType:      row.BindingType,
			ResourceID:       row.ResourceID,
			ParentResourceID: row.ParentResourceID,
		})
	}
	return refs
}

func filterAgentConfigByBindingHealth(config dto.AgentConfigResponse) dto.AgentConfigResponse {
	// Legacy snapshots and test fixtures do not carry binding health. Preserve their
	// existing runtime behavior until the binding-index backfill marks them indexed.
	if config.BindingHealth.Items == nil {
		return config
	}
	active := make(map[string]struct{}, config.BindingHealth.ActiveCount)
	for _, item := range config.BindingHealth.Items {
		if item.Status == agentBindingStatusActive {
			active[agentBindingItemKey(item.BindingType, item.ParentResourceID, item.ResourceID, item.AccessMode)] = struct{}{}
		}
	}
	config.EnabledSkillIDs = filterStringBindings(config.EnabledSkillIDs, agentbindings.BindingTypeSkill, "execute", active)
	config.KnowledgeDatasetIDs = filterStringBindings(config.KnowledgeDatasetIDs, agentbindings.BindingTypeKnowledgeDataset, "read", active)
	databaseBindings := make([]dto.AgentDatabaseBinding, 0, len(config.DatabaseBindings))
	for _, database := range normalizeAgentDatabaseBindings(config.DatabaseBindings) {
		if _, ok := active[agentBindingItemKey(string(agentbindings.BindingTypeDatabase), "", database.DataSourceID, "read")]; !ok {
			continue
		}
		filtered := dto.AgentDatabaseBinding{DataSourceID: database.DataSourceID}
		for _, tableID := range database.TableIDs {
			accessMode := "read"
			if _, writable := stringSet(database.WritableTableIDs)[tableID]; writable {
				accessMode = "write"
			}
			if _, ok := active[agentBindingItemKey(string(agentbindings.BindingTypeDatabaseTable), database.DataSourceID, tableID, accessMode)]; !ok {
				continue
			}
			filtered.TableIDs = append(filtered.TableIDs, tableID)
			if accessMode == "write" {
				filtered.WritableTableIDs = append(filtered.WritableTableIDs, tableID)
			}
		}
		if len(filtered.TableIDs) > 0 {
			databaseBindings = append(databaseBindings, filtered)
		}
	}
	config.DatabaseBindings = databaseBindings
	workflows := make([]dto.AgentWorkflowBinding, 0, len(config.WorkflowBindings))
	for _, workflow := range config.WorkflowBindings {
		if _, ok := active[agentBindingItemKey(string(agentbindings.BindingTypeWorkflow), workflow.AgentID, workflow.BindingID, "execute")]; ok {
			workflows = append(workflows, workflow)
		}
	}
	config.WorkflowBindings = workflows
	return config
}

func filterStringBindings(values []string, bindingType agentbindings.BindingType, accessMode string, active map[string]struct{}) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := active[agentBindingItemKey(string(bindingType), "", value, accessMode)]; ok {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func agentBindingItemKey(bindingType, parentResourceID, resourceID, accessMode string) string {
	return strings.Join([]string{bindingType, parentResourceID, resourceID, accessMode}, "\x00")
}

func agentBindingRevision(rows []agentbindings.Binding) string {
	rows = append([]agentbindings.Binding(nil), rows...)
	sortAgentBindings(rows)
	hash := sha256.New()
	for _, row := range rows {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\n", row.BindingType, row.ParentResourceID, row.ResourceID, row.AccessMode)
	}
	return "br_" + hex.EncodeToString(hash.Sum(nil))
}

func agentBindingHealthRevision(health dto.AgentBindingHealth) string {
	items := append([]dto.AgentBindingHealthItem(nil), health.Items...)
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		return left.BindingType+"\x00"+left.ParentResourceID+"\x00"+left.ResourceID+"\x00"+left.AccessMode <
			right.BindingType+"\x00"+right.ParentResourceID+"\x00"+right.ResourceID+"\x00"+right.AccessMode
	})
	hash := sha256.New()
	for _, item := range items {
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\n", item.BindingType, item.ParentResourceID, item.ResourceID, item.AccessMode, item.Status, item.Reason)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sortAgentBindings(bindings []agentbindings.Binding) {
	sort.Slice(bindings, func(i, j int) bool {
		left := bindings[i]
		right := bindings[j]
		return string(left.BindingType)+"\x00"+left.ParentResourceID+"\x00"+left.ResourceID+"\x00"+left.AccessMode <
			string(right.BindingType)+"\x00"+right.ParentResourceID+"\x00"+right.ResourceID+"\x00"+right.AccessMode
	})
}

func preserveAgentBindingEvidence(bindings, existing []agentbindings.Binding) []agentbindings.Binding {
	byKey := make(map[string]agentbindings.Binding, len(existing))
	for _, binding := range existing {
		byKey[agentBindingItemKey(string(binding.BindingType), binding.ParentResourceID, binding.ResourceID, binding.AccessMode)] = binding
	}
	for idx := range bindings {
		key := agentBindingItemKey(string(bindings[idx].BindingType), bindings[idx].ParentResourceID, bindings[idx].ResourceID, bindings[idx].AccessMode)
		previous, ok := byKey[key]
		if !ok {
			continue
		}
		bindings[idx].AuthorizedBy = cloneBindingUUID(previous.AuthorizedBy)
		if previous.AuthorizedAt != nil {
			authorizedAt := *previous.AuthorizedAt
			bindings[idx].AuthorizedAt = &authorizedAt
		}
	}
	return bindings
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[strings.TrimSpace(value)] = struct{}{}
	}
	return set
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func cloneBindingUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
